package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newOpenAIWSResponseFailedTestConfig() *config.Config {
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.MaxLineSize = defaultMaxLineSize
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.IngressModeDefault = OpenAIWSIngressModeCtxPool
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.QueueLimitPerConn = 8
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3
	return cfg
}

func newOpenAIWSResponseFailedTestAccount(id int64) *Account {
	return &Account{
		ID:          id,
		Name:        "openai-response-failed",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test"},
		Extra:       map[string]any{"responses_websockets_v2_enabled": true},
	}
}

func TestOpenAIWSResponseFailedFailoverRequiresRetryableUncommittedResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{}
	account := newOpenAIWSResponseFailedTestAccount(700)
	newContext := func() *gin.Context {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
		return c
	}
	retryable := []byte(`{"type":"response.failed","response":{"error":{"code":"server_is_overloaded","message":"please retry later"}}}`)
	policy := []byte(`{"type":"response.failed","response":{"error":{"code":"content_policy","message":"request blocked by content policy"}}}`)

	err := svc.newOpenAIWSResponseFailedError(newContext(), account, false, nil, retryable, false, OpenAIUsage{})
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)

	err = svc.newOpenAIWSResponseFailedError(newContext(), account, false, nil, retryable, true, OpenAIUsage{})
	require.False(t, errors.As(err, &failoverErr), "a committed failed event cannot be retried on another account")

	err = svc.newOpenAIWSResponseFailedError(newContext(), account, false, nil, policy, false, OpenAIUsage{})
	require.False(t, errors.As(err, &failoverErr), "request policy rejection must never trigger account failover")
}

func TestOpenAIGatewayService_ForwardWSV2ResponseFailedReturnsErrorWithUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := newOpenAIWSResponseFailedTestConfig()
	failedPayload := []byte(`{"type":"response.failed","response":{"id":"resp_ws_failed","status":"failed","usage":{"input_tokens":11,"output_tokens":2},"error":{"code":"cyber_policy","message":"request blocked by cyber policy"}}}`)
	captureConn := &openAIWSCaptureConn{events: [][]byte{failedPayload}}
	captureDialer := &openAIWSCaptureDialer{conn: captureConn}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(captureDialer)
	svc := &OpenAIGatewayService{
		cfg:              cfg,
		cache:            &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
		openaiWSPool:     pool,
	}
	account := newOpenAIWSResponseFailedTestAccount(701)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)

	result, err := svc.forwardOpenAIWSV2(
		context.Background(),
		c,
		account,
		map[string]any{"model": "gpt-5.1", "stream": false, "input": "hello"},
		"sk-test",
		svc.getOpenAIWSProtocolResolver().Resolve(account),
		false,
		false,
		"gpt-5.1",
		"gpt-5.1",
		time.Now(),
		1,
		"",
	)

	require.Error(t, err)
	require.Contains(t, err.Error(), "upstream response failed")
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "policy rejection must not fail over to another account")
	require.NotNil(t, result)
	require.Equal(t, 11, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
	require.Equal(t, "failed", gjson.GetBytes(rec.Body.Bytes(), "status").String())
	mark := GetOpsCyberPolicy(c)
	require.NotNil(t, mark)
	require.Equal(t, 11, mark.UpstreamInTok)
	require.Equal(t, 2, mark.UpstreamOutTok)
}

func TestOpenAIGatewayService_WSIngressResponseFailedReportsTurnError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, mode := range []string{OpenAIWSIngressModeCtxPool, OpenAIWSIngressModePassthrough} {
		t.Run(mode, func(t *testing.T) {
			cfg := newOpenAIWSResponseFailedTestConfig()
			failedPayload := []byte(`{"type":"response.failed","response":{"id":"resp_ingress_failed","status":"failed","usage":{"input_tokens":7,"output_tokens":1},"error":{"code":"safety_error","message":"request rejected by the safety system"}}}`)
			upstreamConn := &openAIWSCaptureConn{events: [][]byte{failedPayload}}
			captureDialer := &openAIWSCaptureDialer{conn: upstreamConn}
			pool := newOpenAIWSConnPool(cfg)
			pool.setClientDialerForTest(captureDialer)
			svc := &OpenAIGatewayService{
				cfg:                       cfg,
				cache:                     &stubGatewayCache{},
				openaiWSResolver:          NewOpenAIWSProtocolResolver(cfg),
				toolCorrector:             NewCodexToolCorrector(),
				openaiWSPool:              pool,
				openaiWSPassthroughDialer: captureDialer,
			}
			account := newOpenAIWSResponseFailedTestAccount(702)
			account.Extra["openai_apikey_responses_websockets_v2_mode"] = mode

			type turnOutcome struct {
				result *OpenAIForwardResult
				err    error
			}
			turnCh := make(chan turnOutcome, 1)
			hooks := &OpenAIWSIngressHooks{AfterTurn: func(_ int, result *OpenAIForwardResult, turnErr error) {
				turnCh <- turnOutcome{result: result, err: turnErr}
			}}
			serverErrCh := make(chan error, 1)
			wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, acceptErr := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
				if acceptErr != nil {
					serverErrCh <- acceptErr
					return
				}
				defer func() { _ = conn.CloseNow() }()

				readCtx, cancelRead := context.WithTimeout(r.Context(), 3*time.Second)
				msgType, firstMessage, readErr := conn.Read(readCtx)
				cancelRead()
				if readErr != nil {
					serverErrCh <- readErr
					return
				}
				if msgType != coderws.MessageText {
					serverErrCh <- errors.New("first client message was not text")
					return
				}

				rec := httptest.NewRecorder()
				ginCtx, _ := gin.CreateTestContext(rec)
				ginCtx.Request = r.Clone(r.Context())
				serverErrCh <- svc.ProxyResponsesWebSocketFromClient(r.Context(), ginCtx, conn, account, "sk-test", firstMessage, hooks)
			}))
			defer wsServer.Close()

			dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
			clientConn, _, dialErr := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http"), nil)
			cancelDial()
			require.NoError(t, dialErr)
			defer func() { _ = clientConn.CloseNow() }()

			writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
			require.NoError(t, clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","stream":true,"input":"hello"}`)))
			cancelWrite()
			readCtx, cancelEventRead := context.WithTimeout(context.Background(), 3*time.Second)
			_, event, readErr := clientConn.Read(readCtx)
			cancelEventRead()
			require.NoError(t, readErr)
			require.Equal(t, "response.failed", gjson.GetBytes(event, "type").String())
			_ = clientConn.Close(coderws.StatusNormalClosure, "done")

			select {
			case outcome := <-turnCh:
				require.Error(t, outcome.err)
				require.Contains(t, outcome.err.Error(), "upstream response failed")
				require.NotNil(t, outcome.result)
				require.Equal(t, 7, outcome.result.Usage.InputTokens)
				require.Equal(t, 1, outcome.result.Usage.OutputTokens)
			case <-time.After(3 * time.Second):
				t.Fatal("timed out waiting for failed turn callback")
			}

			select {
			case proxyErr := <-serverErrCh:
				if mode == OpenAIWSIngressModeCtxPool {
					require.Error(t, proxyErr)
					require.Contains(t, proxyErr.Error(), "upstream response failed")
				} else if proxyErr != nil {
					require.Contains(t, proxyErr.Error(), "StatusNormalClosure")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("timed out waiting for websocket proxy exit")
			}
		})
	}
}

func TestOpenAIGatewayService_WSHTTPBridgeResponseFailedReturnsError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	failedPayload := `{"type":"response.failed","response":{"id":"resp_bridge_failed","status":"failed","usage":{"input_tokens":5,"output_tokens":0},"error":{"code":"content_policy","message":"request blocked by content policy"}}}`
	sseBody := "data: " + failedPayload + "\n\n"
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(sseBody)),
	}}
	svc := &OpenAIGatewayService{
		cfg:           newOpenAIWSResponseFailedTestConfig(),
		httpUpstream:  upstream,
		toolCorrector: NewCodexToolCorrector(),
	}
	account := newOpenAIWSResponseFailedTestAccount(703)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	var relayed [][]byte

	result, err := svc.proxyOpenAIWSHTTPBridgeTurn(
		context.Background(),
		c,
		account,
		"sk-test",
		[]byte(`{"type":"response.create","model":"gpt-5.1","stream":true,"input":"hello"}`),
		80,
		"gpt-5.1",
		"",
		"",
		"",
		1,
		func(message []byte) error {
			relayed = append(relayed, append([]byte(nil), message...))
			return nil
		},
	)

	require.Error(t, err)
	require.Contains(t, err.Error(), "upstream response failed")
	require.NotNil(t, result)
	require.Equal(t, 5, result.Usage.InputTokens)
	require.Len(t, relayed, 1)
	require.Equal(t, "response.failed", gjson.GetBytes(relayed[0], "type").String())
}
