package handler

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type geminiStickyMigrationWriter struct {
	gin.ResponseWriter
	ctx       *gin.Context
	gateway   *service.GatewayService
	migration *service.GeminiPendingStickyMigration
	once      sync.Once
	stateMu   sync.RWMutex
	commitErr error
	committed bool
}

func installGeminiStickyMigrationWriter(c *gin.Context, gateway *service.GatewayService, migration *service.GeminiPendingStickyMigration) (*geminiStickyMigrationWriter, func()) {
	if c == nil || gateway == nil || migration == nil {
		return nil, func() {}
	}
	original := c.Writer
	writer := &geminiStickyMigrationWriter{ResponseWriter: original, ctx: c, gateway: gateway, migration: migration}
	c.Writer = writer
	return writer, func() { c.Writer = original }
}

func (w *geminiStickyMigrationWriter) commit() error {
	if w == nil {
		return nil
	}
	w.once.Do(func() {
		commitErr := w.gateway.CommitGeminiStickyMigration(w.ctx.Request.Context(), w.migration)
		w.stateMu.Lock()
		w.commitErr = commitErr
		w.committed = commitErr == nil
		w.stateMu.Unlock()
	})
	return w.CommitError()
}

func (w *geminiStickyMigrationWriter) CommitError() error {
	if w == nil {
		return nil
	}
	w.stateMu.RLock()
	defer w.stateMu.RUnlock()
	return w.commitErr
}

func (w *geminiStickyMigrationWriter) Committed() bool {
	if w == nil {
		return false
	}
	w.stateMu.RLock()
	defer w.stateMu.RUnlock()
	return w.committed
}

func (w *geminiStickyMigrationWriter) WriteHeader(code int) {
	if code < http.StatusBadRequest {
		if err := w.commit(); err != nil {
			w.ResponseWriter.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprintf(w.ResponseWriter, "Gemini session migration failed: %v", err)
			return
		}
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *geminiStickyMigrationWriter) WriteHeaderNow() {
	status := w.Status()
	if status == 0 {
		status = http.StatusOK
	}
	if status < http.StatusBadRequest {
		if err := w.commit(); err != nil {
			w.ResponseWriter.WriteHeader(http.StatusServiceUnavailable)
			return
		}
	}
	w.ResponseWriter.WriteHeaderNow()
}

func (w *geminiStickyMigrationWriter) Write(data []byte) (int, error) {
	if err := w.CommitError(); err != nil {
		return 0, err
	}
	if status := w.Status(); status == 0 || status < http.StatusBadRequest {
		if err := w.commit(); err != nil {
			return 0, err
		}
	}
	return w.ResponseWriter.Write(data)
}

func (w *geminiStickyMigrationWriter) WriteString(data string) (int, error) {
	if err := w.CommitError(); err != nil {
		return 0, err
	}
	if status := w.Status(); status == 0 || status < http.StatusBadRequest {
		if err := w.commit(); err != nil {
			return 0, err
		}
	}
	return w.ResponseWriter.WriteString(data)
}

func (w *geminiStickyMigrationWriter) Flush() {
	status := w.Status()
	if status >= http.StatusBadRequest || w.commit() == nil {
		w.ResponseWriter.Flush()
	}
}

func finishGeminiStickyMigration(
	ctx context.Context,
	gateway *service.GatewayService,
	migration *service.GeminiPendingStickyMigration,
	writer *geminiStickyMigrationWriter,
	result *service.ForwardResult,
	forwardErr error,
) (*service.ForwardResult, error) {
	if writer != nil && writer.CommitError() != nil && forwardErr == nil {
		forwardErr = writer.CommitError()
		result = nil
	}
	if forwardErr == nil && migration != nil {
		if err := gateway.CommitGeminiStickyMigration(ctx, migration); err != nil {
			forwardErr = err
			result = nil
		}
	}
	if forwardErr != nil && migration != nil {
		gateway.AbortGeminiStickyMigration(ctx, migration)
	}
	return result, forwardErr
}
