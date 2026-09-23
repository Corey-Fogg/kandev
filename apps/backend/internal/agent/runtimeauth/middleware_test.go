package runtimeauth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/auth/authn"
)

func TestMiddlewareLeavesResolvedUserBearersToTheUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name     string
		identity *authn.Identity
		want     int
	}{
		{name: "personal access token", identity: &authn.Identity{UserID: "user", TokenID: "pat"}, want: http.StatusOK},
		{name: "auth disabled", identity: &authn.Identity{UserID: "local", Synthetic: true}, want: http.StatusUnauthorized},
		{name: "unresolved bearer", want: http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				if tc.identity != nil {
					authn.SetOnGin(c, *tc.identity)
				}
			})
			router.GET("/", Middleware(NewAgentAuth(""), nil), func(c *gin.Context) { c.Status(http.StatusOK) })
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Authorization", "Bearer kdv_not_a_runtime_jwt")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d", w.Code, tc.want)
			}
		})
	}
}
