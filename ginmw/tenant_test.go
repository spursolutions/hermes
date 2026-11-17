package ginmw

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/spursolutions/hermes/tenancy"
)

func probe(c *gin.Context) {
	id, ok := tenancy.IDFromContext(c.Request.Context())
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "no tenant in context"})
		return
	}
	raw, _ := tenancy.RawHeaderFromContext(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{"tenant": id, "gin_tenant": c.GetString(ContextKey), "raw": raw})
}

func serve(r *gin.Engine, header string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	if header != "" {
		req.Header.Set(tenancy.HeaderName, header)
	}
	r.ServeHTTP(w, req)
	return w
}

func TestTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	catalog := tenancy.NewCatalog(map[string]string{"acme": "AcmeDB"})

	r := gin.New()
	r.Use(Tenant(catalog))
	r.GET("/probe", probe)

	tests := []struct {
		name     string
		header   string
		wantCode int
		wantBody string
	}{
		{name: "known tenant flows through", header: "acme|user123", wantCode: http.StatusOK,
			wantBody: `{"gin_tenant":"acme","raw":"acme|user123","tenant":"acme"}`},
		{name: "missing header rejected", header: "", wantCode: http.StatusBadRequest,
			wantBody: `{"error":"tenant identity is required"}`},
		{name: "missing tenant segment rejected", header: "|user123", wantCode: http.StatusBadRequest,
			wantBody: `{"error":"tenant identity is required"}`},
		{name: "unknown tenant rejected", header: "rogue|user123", wantCode: http.StatusBadRequest,
			wantBody: `{"error":"unknown tenant: rogue"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := serve(r, tt.header)
			if w.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d (body %s)", w.Code, tt.wantCode, w.Body)
			}
			if w.Body.String() != tt.wantBody {
				t.Fatalf("body = %s, want %s", w.Body, tt.wantBody)
			}
		})
	}
}

func TestTenant_WithErrorHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var got error

	r := gin.New()
	r.Use(Tenant(tenancy.NewCatalog(map[string]string{"acme": "AcmeDB"}),
		WithErrorHandler(func(c *gin.Context, err error) {
			got = err
			c.Status(http.StatusTeapot)
		})))
	r.GET("/probe", probe)

	w := serve(r, "rogue|user123")
	if w.Code != http.StatusTeapot {
		t.Fatalf("status = %d, want custom handler's 418", w.Code)
	}
	var unknown *tenancy.UnknownTenantError
	if !errors.As(got, &unknown) || unknown.TenantID != "rogue" {
		t.Fatalf("handler got %v, want *UnknownTenantError for rogue", got)
	}
}

func TestTenant_WithOnResolved(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var resolved string

	r := gin.New()
	r.Use(Tenant(tenancy.NewCatalog(map[string]string{"acme": "AcmeDB"}),
		WithOnResolved(func(_ *gin.Context, id string) { resolved = id })))
	r.GET("/probe", probe)

	if w := serve(r, "acme|user123"); w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if resolved != "acme" {
		t.Fatalf("hook got %q, want acme", resolved)
	}

	resolved = ""
	serve(r, "rogue|user123")
	if resolved != "" {
		t.Fatal("hook must not run for rejected requests")
	}
}
