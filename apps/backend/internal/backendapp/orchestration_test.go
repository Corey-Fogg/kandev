package backendapp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/config"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
)

func TestOrchestrationFlagGuardsLegacyRoutes(t *testing.T) {
	a, svc, repo, conversation := coordinatorConversationFixture(t)
	database := sqlx.NewDb(a.taskRepo.DB(), "sqlite3")
	office, err := officesqlite.NewWithDB(database, database, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{false, true} {
		router := gin.New()
		p := routeParams{features: config.FeaturesConfig{Orchestration: enabled, Office: !enabled}, officeRepo: office, orchestrationRepo: repo, taskSvc: svc}
		g := router.Group("/api/v1/office", orchestrationCompatibilityGate(p))
		g.GET("/tasks/:id", func(c *gin.Context) { c.Status(200) })
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/office/tasks/"+conversation, nil))
		if w.Code != http.StatusNotFound {
			t.Fatalf("conversation flag %v: %d", enabled, w.Code)
		}
	}
}
