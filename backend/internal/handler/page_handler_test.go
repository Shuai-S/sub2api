package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type pageSettingRepo struct {
	customMenuItems string
}

func (r *pageSettingRepo) Get(context.Context, string) (*service.Setting, error) {
	return nil, service.ErrSettingNotFound
}

func (r *pageSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	if key == service.SettingKeyCustomMenuItems {
		return r.customMenuItems, nil
	}
	return "", service.ErrSettingNotFound
}

func (r *pageSettingRepo) Set(context.Context, string, string) error { return nil }

func (r *pageSettingRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{}, nil
}

func (r *pageSettingRepo) SetMultiple(context.Context, map[string]string) error { return nil }
func (r *pageSettingRepo) GetAll(context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}
func (r *pageSettingRepo) Delete(context.Context, string) error { return nil }

func runCustomMenuModalRequest(t *testing.T, raw, id, role string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc := service.NewSettingService(&pageSettingRepo{customMenuItems: raw}, &config.Config{})
	h := NewPageHandler(t.TempDir(), svc)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/custom-menu-items/"+id+"/modal", nil)
	c.Params = gin.Params{{Key: "id", Value: id}}
	if role != "" {
		c.Set(string(middleware.ContextKeyUserRole), role)
	}
	h.GetCustomMenuModal(c)
	return rec
}

func TestGetCustomMenuModalChecksPlacementAndVisibility(t *testing.T) {
	raw := `[
		{"id":"notice","label":"Notice","visibility":"user","placement":"header","modal_content":"# Body"},
		{"id":"admin-notice","label":"Admin","visibility":"admin","placement":"header","modal_title":"Admin title","modal_content":"Secret"},
		{"id":"sidebar","label":"Sidebar","visibility":"user","placement":"sidebar","url":"https://example.com"}
	]`

	userResponse := runCustomMenuModalRequest(t, raw, "notice", service.RoleUser)
	require.Equal(t, http.StatusOK, userResponse.Code)
	require.Contains(t, userResponse.Body.String(), `"title":"Notice"`)
	require.Contains(t, userResponse.Body.String(), `"content":"# Body"`)

	deniedResponse := runCustomMenuModalRequest(t, raw, "admin-notice", service.RoleUser)
	require.Equal(t, http.StatusNotFound, deniedResponse.Code)

	adminResponse := runCustomMenuModalRequest(t, raw, "admin-notice", service.RoleAdmin)
	require.Equal(t, http.StatusOK, adminResponse.Code)
	require.Contains(t, adminResponse.Body.String(), `"title":"Admin title"`)

	sidebarResponse := runCustomMenuModalRequest(t, raw, "sidebar", service.RoleAdmin)
	require.Equal(t, http.StatusNotFound, sidebarResponse.Code)
}

func TestCleanPageImageRelativePath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{name: "single filename", in: "logo.png", want: "logo.png", ok: true},
		{name: "nested path", in: "images/logo.png", want: filepath.Join("images", "logo.png"), ok: true},
		{name: "dot prefix", in: "./logo.png", want: "logo.png", ok: true},
		{name: "url escaped slash", in: "images%2Flogo.png", want: filepath.Join("images", "logo.png"), ok: true},
		{name: "parent traversal", in: "../secret.png", ok: false},
		{name: "encoded parent traversal", in: "%2e%2e/secret.png", ok: false},
		{name: "backslash traversal", in: `images\secret.png`, ok: false},
		{name: "absolute path", in: "/etc/passwd", ok: false},
		{name: "encoded absolute path", in: "%2fetc/passwd", ok: false},
		{name: "encoded nul byte", in: "logo.png%00", ok: false},
		{name: "invalid escape", in: "logo.png%zz", ok: false},
		{name: "empty path", in: "", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := cleanPageImageRelativePath(tt.in)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Fatalf("path = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolvePageImagePath(t *testing.T) {
	root := t.TempDir()
	pagesDir := filepath.Join(root, "pages")
	base := filepath.Join(pagesDir, "guide")
	if err := os.MkdirAll(filepath.Join(base, "images"), 0755); err != nil {
		t.Fatalf("create images dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base, "logo.png"), []byte("fake"), 0644); err != nil {
		t.Fatalf("create direct image: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base, "images", "logo.png"), []byte("fake"), 0644); err != nil {
		t.Fatalf("create image: %v", err)
	}

	got, ok := resolvePageImagePath(pagesDir, base, "logo.png")
	if !ok {
		t.Fatal("expected direct image path to be accepted")
	}
	want := mustEvalSymlinks(t, filepath.Join(base, "logo.png"))
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}

	got, ok = resolvePageImagePath(pagesDir, base, "images/logo.png")
	if !ok {
		t.Fatal("expected nested image path to be accepted")
	}
	want = mustEvalSymlinks(t, filepath.Join(base, "images", "logo.png"))
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}

	if got, ok := resolvePageImagePath(pagesDir, base, "../guide.md"); ok {
		t.Fatalf("expected traversal to be rejected, got %q", got)
	}
}

func TestResolvePageImagePathRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	pagesDir := filepath.Join(root, "pages")
	base := filepath.Join(pagesDir, "guide")
	outside := filepath.Join(root, "outside")

	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatalf("create page dir: %v", err)
	}
	if err := os.MkdirAll(outside, 0755); err != nil {
		t.Fatalf("create outside dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.png"), []byte("secret"), 0644); err != nil {
		t.Fatalf("create outside file: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(base, "images")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	if got, ok := resolvePageImagePath(pagesDir, base, "images/secret.png"); ok {
		t.Fatalf("expected symlink escape to be rejected, got %q", got)
	}
}

func mustEvalSymlinks(t *testing.T, path string) string {
	t.Helper()

	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("eval symlinks for %q: %v", path, err)
	}
	return realPath
}
