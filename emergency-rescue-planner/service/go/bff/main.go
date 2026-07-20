// @title           Emergency Rescue Planner API
// @version         1.0
// @description     Backend API for the emergency rescue planner
// @host            localhost:8080
// @BasePath        /

package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	httpSwagger "github.com/swaggo/http-swagger"

	_ "bff/docs"
	active_route_engine "bff/internal/active-route-engine"
	checklist_engine "bff/internal/checklist-engine"
	cluster_engine "bff/internal/cluster-engine"
	conditions_engine "bff/internal/conditions-engine"
	"bff/internal/database"
	demo_engine "bff/internal/demo-engine"
	external "bff/internal/external-api-manager"
	field_report_engine "bff/internal/field-report-engine"
	geocode_engine "bff/internal/geocode-engine"
	"bff/internal/handler"
	"bff/internal/health"
	info_engine "bff/internal/info-engine"
	knowledge_engine "bff/internal/knowledge-engine"
	"bff/internal/middleware"
	persons_engine "bff/internal/persons-engine"
	route_anchor_engine "bff/internal/route-anchor-engine"
	route_engine "bff/internal/route-engine"
	translation_engine "bff/internal/translation-engine"
	voice_search_engine "bff/internal/voice-search-engine"
)

// Version is the public service version. Override at build time via
// `-ldflags "-X main.Version=$(git rev-parse --short HEAD)"`.
var Version = "dev"

// main wires up engines, registers HTTP routes, and starts the server.
func main() {
	_ = godotenv.Load() // Pass this if not found, since this is only for local testing

	startedAt := time.Now()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Assign the server
	mux := http.NewServeMux()

	// Capture each pattern alongside its mux registration so /health_check
	// can return the full route table.
	var registeredRoutes []health.Route
	register := func(pattern string, handler func(http.ResponseWriter, *http.Request)) {
		registeredRoutes = append(registeredRoutes, health.ParseRoutePattern(pattern))
		mux.HandleFunc(pattern, handler)
	}

	// Swag
	if os.Getenv("ENV") != "deployment" {
		mux.HandleFunc("/swagger/", httpSwagger.WrapHandler)
	}

	// Create database connector
	db, err := database.NewDBConnector(database.DBConnectorConfig{
		DSN:            os.Getenv("DATABASE_URL"),
		MaxOpenConns:   20,
		ConnectTimeOut: 5 * time.Second,
	})
	if err != nil {
		log.Fatalf("Failed to initialize the database connector: %v", err)
	}

	// Create Info engine
	engine := info_engine.NewInfoEngine(db)

	// Create Knowledge engine (mock-backed for iteration 1; swap NewMockRepository
	// for a Postgres-backed Repository when the knowledge_articles table lands).
	repo := knowledge_engine.NewMockRepository()
	kEngine := knowledge_engine.NewKnowledgeEngine(repo)

	// Create Checklist engine (Postgres-backed via *DBConnector.GetChecklist)
	chEngine := checklist_engine.NewChecklistEngine(db)

	// Create Route engine (ORS-backed; 503 when ORS_API_KEY unset)
	rEngine := route_engine.NewRouteEngine(
		os.Getenv("ORS_API_KEY"),
		route_engine.NewORSClient(os.Getenv("ORS_API_KEY")),
		route_engine.NewMockZoneStore(),
	)

	// Create Conditions engine + start Hub (wall-clock scheduler opt-in via /api/demo/hazard-demo/start)
	condRepo := conditions_engine.NewMockZoneRepository()
	cEngine := conditions_engine.NewConditionsEngine(condRepo)
	cEngine.StartHub(ctx)

	// Create Persons engine
	personRepo := persons_engine.NewMockPersonRepository()
	pEngine := persons_engine.NewPersonsEngine(personRepo)

	// Create Field report engine + Cluster engine (D7 cluster-to-hazard promotion)
	frEngine := field_report_engine.NewFieldReportEngine()
	clEngine := cluster_engine.NewClusterEngine(frEngine.Store(), cEngine)
	frEngine.SetClusterNotifier(clEngine)

	// Create Demo engine (per-session heartbeat progress store; read by route-anchor-engine)
	demoStore := demo_engine.NewInMemoryDemoSessionStore()
	deEngine := demo_engine.NewDemoEngine(demoStore)

	// Create Active route engine and wire reroute dependencies.
	arRepo := active_route_engine.NewMockActiveRouteRepository()
	arEngine := active_route_engine.NewActiveRouteEngine(arRepo)
	arEngine.SetRerouteComponents(route_engine.NewORSClient(os.Getenv("ORS_API_KEY")), condRepo, cEngine.Hub())
	arEngine.SetDemoStore(demoStore)
	cEngine.SetHazardHook(arEngine.OnHazardActivated)

	// Create Route anchor engine (feature 7: spawns hazards/persons at route-progress anchors)
	anchorEngine := route_anchor_engine.NewRouteAnchorEngine(
		route_anchor_engine.DefaultHazardScript,
		route_anchor_engine.DefaultPersonScript,
		ctx,
		condRepo,
		cEngine,
		cEngine.Hub(),
		arRepo,
		personRepo,
		demoStore,
	)
	anchorEngine.Start(ctx)
	deEngine.SetHeartbeatHook(anchorEngine.OnHeartbeat)
	arEngine.SetRouteDeletedHook(anchorEngine.OnRouteDeleted)
	arEngine.SetRerouteAcceptedHook(anchorEngine.OnRerouteAccepted)
	anchorEngine.SetRouteRecomputeHook(arEngine.OnHazardActivated)

	// Create Geocode engine (Nominatim proxy; 1 req/s rate limit enforced internally)
	gClient := geocode_engine.NewNominatimClient()
	gEngine := geocode_engine.NewGeocodeEngine(gClient)

	// Create External API manager (OpenAI / Anthropic). Non-fatal: if neither API
	// key is set, the BFF starts without AI routes so other features keep working.
	var aiMgr *external.ExternalAPIManager
	if mgr, err := external.NewExternalAPIManager(external.Config{
		OpenAIKey:    os.Getenv("OPENAI_API_KEY"),
		AnthropicKey: os.Getenv("ANTHROPIC_API_KEY"),
		Timeout:      60 * time.Second,
		MaxRetry:     2,
	}); err != nil {
		log.Printf("External API manager disabled: %v", err)
	} else {
		aiMgr = mgr
	}

	// Assign the routine
	register("/ws", handler.WsHandler)
	register("/api/overview/geogroup", engine.GetGeoAreas)
	register("/api/overview/facilities", engine.GetFacilities)

	register("POST /api/route/calculate", rEngine.CalculateRoute)

	register("GET /api/conditions/risk-zones", cEngine.GetRiskZones)
	register("GET /ws/hazards", cEngine.WsHazards)

	register("GET /api/vulnerable-persons", pEngine.GetVulnerablePersons)

	register("GET /api/field-reports", frEngine.GetReports)
	register("POST /api/field-reports", frEngine.SubmitReport)
	register("POST /api/field-reports/{id}/confirm", frEngine.ConfirmReport)

	register("POST /api/demo/heartbeat", deEngine.Heartbeat)
	register("POST /api/demo/hazard-demo/start", cEngine.StartHazardDemoHTTP)
	register("POST /api/demo/hazard-demo/stop", cEngine.StopHazardDemoHTTP)
	register("POST /api/demo/route-anchor/clear", anchorEngine.ClearSessionHTTP)
	register("POST /api/demo/origin-anchor/spawn", anchorEngine.SpawnOriginAnchorsHTTP)
	register("POST /api/demo/origin-anchor/clear", anchorEngine.ClearOriginAnchorsHTTP)
	register("POST /api/demo/origin-anchor/auto-expand", anchorEngine.SetOriginAnchorAutoExpandHTTP)

	register("POST /api/routes/active", arEngine.RegisterActiveRoute)
	register("DELETE /api/routes/active/{active_route_id}", arEngine.DeleteActiveRoute)
	register("POST /api/routes/active/{active_route_id}/accept-reroute", arEngine.AcceptReroute)

	register("GET /api/geocode/search", gEngine.Search)
	register("GET /api/geocode/reverse", gEngine.Reverse)

	register("GET /api/knowledge/search", kEngine.Search)
	register("GET /api/knowledge/articles/{id}", kEngine.GetByID)

	register("GET /api/checklist", chEngine.GetChecklist)

	if aiMgr != nil {
		register("/api/ai/chat", aiMgr.ChatHTTP)
		register("/api/ai/stream", aiMgr.StreamChat)

		tEngine := translation_engine.NewTranslationEngine(aiMgr)
		register("POST /api/knowledge/transcribe", tEngine.Transcribe)
		register("POST /api/knowledge/translate", tEngine.Translate)
		register("POST /api/knowledge/tts", tEngine.TTS)
		register("GET /api/knowledge/languages", tEngine.Languages)

		vsEngine := voice_search_engine.NewVoiceSearchEngine(aiMgr, repo)
		register("POST /api/knowledge/voice-search", vsEngine.VoiceSearch)
	}

	// Add /health_check to the snapshot before the engine reads the slice.
	registeredRoutes = append(registeredRoutes, health.Route{Method: "GET", Path: "/health_check"})
	hEngine := health.NewHealthEngine(health.Config{
		ServiceID:   "salus-bridge-bff",
		Description: "Salus Bridge Backend-for-Frontend (Go)",
		Version:     Version,
		StartedAt:   startedAt,
		DBPinger:    db,
		EnvSnapshot: health.EnvSnapshot{
			OpenAIKey:    os.Getenv("OPENAI_API_KEY") != "",
			AnthropicKey: os.Getenv("ANTHROPIC_API_KEY") != "",
			ORSKey:       os.Getenv("ORS_API_KEY") != "",
		},
		Routes: registeredRoutes,
	})
	mux.HandleFunc("GET /health_check", hEngine.GetHealth)

	// Start the server
	if err := http.ListenAndServe(":"+os.Getenv("GO_SERVICE_PORT"), middleware.CorsMiddleware(mux)); err != nil {
		log.Fatal(err) // The server failed to start
	}
}
