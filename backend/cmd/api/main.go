// Command api runs the single-process newsroom HTTP API.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	_ "modernc.org/sqlite"
	"newsroom/internal/agent"
	appconfig "newsroom/internal/config"
	"newsroom/internal/providers/exa"
	"newsroom/internal/providers/openrouter"
	"newsroom/migrations"
)

type config struct {
	appEnv, addr, dbPath                         string
	cors                                         []string
	exaAPIKey, openRouterAPIKey, openRouterModel string
	openRouterMaxOutputTokens                    int
	schedulerEnabled                             bool
	runTimeout                                   time.Duration
	queueCapacity                                int
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func loadConfig() (config, error) {
	schedulerEnabled, err := strconv.ParseBool(env("AGENT_SCHEDULER_ENABLED", "false"))
	if err != nil {
		return config{}, fmt.Errorf("AGENT_SCHEDULER_ENABLED must be true or false: %w", err)
	}
	runTimeout, err := time.ParseDuration(env("AGENT_RUN_TIMEOUT", "2m"))
	if err != nil || runTimeout <= 0 {
		return config{}, fmt.Errorf("AGENT_RUN_TIMEOUT must be a positive duration")
	}
	queueCapacity, err := strconv.Atoi(env("AGENT_QUEUE_CAPACITY", "8"))
	if err != nil || queueCapacity < 1 {
		return config{}, fmt.Errorf("AGENT_QUEUE_CAPACITY must be a positive integer")
	}
	openRouterMaxOutputTokens, err := strconv.Atoi(env("OPENROUTER_MAX_OUTPUT_TOKENS", "4096"))
	if err != nil || openRouterMaxOutputTokens < 1 {
		return config{}, fmt.Errorf("OPENROUTER_MAX_OUTPUT_TOKENS must be a positive integer")
	}
	origins := strings.Split(env("CORS_ALLOWED_ORIGINS", "http://localhost:8081,http://localhost:19006"), ",")
	return config{
		appEnv:                    env("APP_ENV", "development"),
		addr:                      env("HTTP_ADDR", ":8080"),
		dbPath:                    env("DATABASE_PATH", "./data/newsroom.db"),
		cors:                      origins,
		exaAPIKey:                 os.Getenv("EXA_API_KEY"),
		openRouterAPIKey:          os.Getenv("OPENROUTER_API_KEY"),
		openRouterModel:           os.Getenv("OPENROUTER_MODEL"),
		schedulerEnabled:          schedulerEnabled,
		runTimeout:                runTimeout,
		queueCapacity:             queueCapacity,
		openRouterMaxOutputTokens: openRouterMaxOutputTokens,
	}, nil
}

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if _, err := appconfig.LoadDotEnv(); err != nil {
		log.Error("load local environment", "error", err)
		os.Exit(1)
	}
	cfg, err := loadConfig()
	if err != nil {
		log.Error("load configuration", "error", err)
		os.Exit(1)
	}
	log.Info("configuration loaded", "environment", cfg.appEnv, "scheduler_enabled", cfg.schedulerEnabled)
	if err := os.MkdirAll(filepath.Dir(cfg.dbPath), 0o755); err != nil {
		log.Error("create database directory", "error", err)
		os.Exit(1)
	}
	db, err := openDatabase(cfg.dbPath)
	if err != nil {
		log.Error("open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	if err := migrate(db); err != nil {
		log.Error("migrate database", "error", err)
		os.Exit(1)
	}
	if err := seed(db); err != nil {
		log.Error("seed database", "error", err)
		os.Exit(1)
	}
	repo := repository{db: db}
	if err := repo.recoverInterruptedRuns(); err != nil {
		log.Error("recover interrupted runs", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	configured := cfg.exaAPIKey != "" && cfg.openRouterAPIKey != "" && cfg.openRouterModel != ""
	writer := openrouter.New(cfg.openRouterAPIKey, cfg.openRouterModel, nil)
	writer.MaxOutputTokens = cfg.openRouterMaxOutputTokens
	writer.Logger = log
	workflow := agent.Workflow{Researcher: exa.New(cfg.exaAPIKey, nil), Writer: writer}
	queue := newRunQueue(ctx, repo, workflow, cfg.runTimeout, cfg.queueCapacity, configured, log)
	var scheduler *scheduler
	if cfg.schedulerEnabled {
		if !configured {
			log.Warn("automatic scheduling disabled because research providers are not configured")
		} else {
			scheduler, err = newScheduler(ctx, repo, queue, log)
			if err != nil {
				log.Error("start scheduler", "error", err)
				queue.stop(context.Background())
				os.Exit(1)
			}
			log.Info("automatic research scheduling enabled")
		}
	}

	h := newAPI(db, cfg.cors, log, queue)
	server := &http.Server{Addr: cfg.addr, Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	go func() {
		log.Info("API listening", "addr", cfg.addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server stopped", "error", err)
			os.Exit(1)
		}
	}()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if scheduler != nil {
		scheduler.stop()
	}
	queue.stop(shutdown)
	_ = server.Shutdown(shutdown)
}

func openDatabase(path string) (*sql.DB, error) {
	// MaxOpenConns=1 makes the single-process connection strategy explicit. The
	// DSN pragmas are then applied to the only connection used by this process.
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func jsonError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func jsonResponse(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func notImplemented(w http.ResponseWriter) {
	jsonError(w, http.StatusNotImplemented, "not_implemented", "This operation is not implemented yet.")
}

// migrate applies each numbered SQL file once and records its version. Existing
// databases from the first scaffold are safe because the SQL is idempotent.
func migrate(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TEXT NOT NULL)`)
	if err != nil {
		return err
	}
	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		var applied int
		if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, entry.Name()).Scan(&applied); err != nil {
			return err
		}
		if applied > 0 {
			continue
		}
		sqlBytes, err := migrations.FS.ReadFile(entry.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", entry.Name(), err)
		}
		if _, err := tx.Exec(string(sqlBytes)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", entry.Name(), err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`, entry.Name(), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", entry.Name(), err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func seed(db *sql.DB) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	agents := []struct{ id, assignment string }{{"premier_league", "Premier League news"}, {"bundesliga", "Bundesliga transfers"}, {"coach_statements", "Coach statements"}}
	for _, a := range agents {
		_, err := db.Exec(`INSERT INTO agents (id, assignment, language, platforms, enabled, research_interval_seconds, created_at, updated_at) VALUES (?, ?, 'fr', '["facebook","x"]', 1, 1800, ?, ?) ON CONFLICT(id) DO NOTHING`, a.id, a.assignment, now, now)
		if err != nil {
			return err
		}
	}
	return nil
}

type api struct {
	db    *sql.DB
	cors  map[string]bool
	log   *slog.Logger
	queue *runQueue
}

func newAPI(db *sql.DB, origins []string, log *slog.Logger, queue *runQueue) http.Handler {
	cors := map[string]bool{}
	for _, origin := range origins {
		cors[strings.TrimSpace(origin)] = true
	}
	a := &api{db: db, cors: cors, log: log, queue: queue}
	return a.middleware(http.HandlerFunc(a.route))
}
func (a *api) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		origin := r.Header.Get("Origin")
		if a.cors[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PATCH,OPTIONS")
		}
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		defer func() {
			if recovered := recover(); recovered != nil {
				a.log.Error("panic recovered", "error", recovered)
				jsonError(w, 500, "internal_error", "Internal server error.")
			}
		}()
		started := time.Now()
		next.ServeHTTP(w, r)
		a.log.Info("request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(started))
	})
}
func (a *api) route(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(path, "/")
	if r.Method == "GET" && path == "health" {
		jsonResponse(w, 200, map[string]string{"status": "ok"})
		return
	}
	if len(parts) >= 3 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "agents" {
		a.agents(w, r, parts[3:])
		return
	}
	if len(parts) >= 3 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "drafts" && r.Method == "PATCH" {
		if len(parts) != 4 || parts[3] == "" {
			jsonError(w, http.StatusNotFound, "not_found", "Route not found.")
			return
		}
		a.patchDraft(w, r, parts[3])
		return
	}
	jsonError(w, 404, "not_found", "Route not found.")
}

type draftPatch struct {
	headline, facebookText, xText, reviewStatus *string
}

func (a *api) patchDraft(w http.ResponseWriter, r *http.Request, rawID string) {
	draftID, err := url.PathUnescape(rawID)
	if err != nil || strings.TrimSpace(draftID) == "" {
		jsonError(w, http.StatusBadRequest, "invalid_request", "Invalid draft identifier.")
		return
	}
	patch, err := decodeDraftPatch(r.Body)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	_, err = a.applyDraftPatch(draftID, patch)
	if errors.Is(err, sql.ErrNoRows) {
		jsonError(w, http.StatusNotFound, "not_found", "Draft not found.")
		return
	}
	if err != nil {
		a.log.Error("update draft", "draft_id", draftID, "error", err)
		jsonError(w, http.StatusInternalServerError, "database_error", "Could not update draft.")
		return
	}
	draft, err := a.draftPayload(draftID)
	if err != nil {
		a.log.Error("read updated draft", "draft_id", draftID, "error", err)
		jsonError(w, http.StatusInternalServerError, "database_error", "Could not read updated draft.")
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"draft": draft})
}

func decodeDraftPatch(body io.Reader) (draftPatch, error) {
	var raw map[string]json.RawMessage
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(&raw); err != nil {
		return draftPatch{}, fmt.Errorf("Request body must be a JSON object.")
	}
	if raw == nil || len(raw) == 0 {
		return draftPatch{}, fmt.Errorf("Request body must include at least one editable field.")
	}
	if err := ensureEOF(decoder); err != nil {
		return draftPatch{}, fmt.Errorf("Request body must contain one JSON object.")
	}
	patch := draftPatch{}
	for field, value := range raw {
		parsed, err := requiredPatchString(field, value)
		if err != nil {
			return draftPatch{}, err
		}
		switch field {
		case "headline":
			patch.headline = &parsed
		case "facebookText":
			patch.facebookText = &parsed
		case "xText":
			if utf8.RuneCountInString(parsed) > 280 {
				return draftPatch{}, fmt.Errorf("xText must be 280 Unicode characters or fewer.")
			}
			patch.xText = &parsed
		case "reviewStatus":
			if parsed != "pending" && parsed != "approved" && parsed != "rejected" {
				return draftPatch{}, fmt.Errorf("reviewStatus must be pending, approved, or rejected.")
			}
			patch.reviewStatus = &parsed
		default:
			return draftPatch{}, fmt.Errorf("Unknown field %q.", field)
		}
	}
	return patch, nil
}

func requiredPatchString(field string, raw json.RawMessage) (string, error) {
	if string(raw) == "null" {
		return "", fmt.Errorf("%s must not be null.", field)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("%s must be a string.", field)
	}
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s must not be blank.", field)
	}
	return value, nil
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func (a *api) applyDraftPatch(draftID string, patch draftPatch) (bool, error) {
	tx, err := a.db.Begin()
	if err != nil {
		return false, err
	}
	rollback := func(cause error) (bool, error) { _ = tx.Rollback(); return false, cause }
	var headline, facebookText, xText, reviewStatus string
	if err := tx.QueryRow(`SELECT headline, facebook_text, x_text, review_status FROM drafts WHERE id=?`, draftID).Scan(&headline, &facebookText, &xText, &reviewStatus); err != nil {
		return rollback(err)
	}
	updatedHeadline, updatedFacebook, updatedX, updatedStatus := headline, facebookText, xText, reviewStatus
	textEdited := patch.headline != nil || patch.facebookText != nil || patch.xText != nil
	if patch.headline != nil {
		updatedHeadline = *patch.headline
	}
	if patch.facebookText != nil {
		updatedFacebook = *patch.facebookText
	}
	if patch.xText != nil {
		updatedX = *patch.xText
	}
	if patch.reviewStatus != nil {
		updatedStatus = *patch.reviewStatus
	} else if textEdited && reviewStatus != "pending" {
		updatedStatus = "pending"
	}
	if updatedHeadline == headline && updatedFacebook == facebookText && updatedX == xText && updatedStatus == reviewStatus {
		if err := tx.Commit(); err != nil {
			return false, err
		}
		return false, nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.Exec(`UPDATE drafts SET headline=?, facebook_text=?, x_text=?, review_status=?, updated_at=? WHERE id=?`, updatedHeadline, updatedFacebook, updatedX, updatedStatus, now, draftID); err != nil {
		return rollback(err)
	}
	if updatedHeadline != headline {
		if _, err := tx.Exec(`UPDATE messages SET text=? WHERE draft_id=? AND message_type='draft'`, updatedHeadline, draftID); err != nil {
			return rollback(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
func (a *api) agents(w http.ResponseWriter, r *http.Request, rest []string) {
	if len(rest) == 0 && r.Method == "GET" {
		rows, err := a.db.Query(`SELECT a.id, a.assignment, a.language, a.platforms, a.enabled, a.research_interval_seconds, a.created_at, a.updated_at,
			(SELECT COUNT(*) FROM drafts d WHERE d.agent_id = a.id AND d.review_status = 'pending'),
			(SELECT COUNT(*) FROM runs r WHERE r.agent_id = a.id AND r.status IN ('queued', 'running'))
			FROM agents a ORDER BY a.id`)
		if err != nil {
			jsonError(w, 500, "database_error", "Could not list agents.")
			return
		}
		defer rows.Close()
		result := []map[string]any{}
		for rows.Next() {
			var id, assignment, language, platforms, created, updated string
			var enabled, interval, pending, running int
			if err := rows.Scan(&id, &assignment, &language, &platforms, &enabled, &interval, &created, &updated, &pending, &running); err != nil {
				jsonError(w, 500, "database_error", "Could not read agents.")
				return
			}
			var platformList []string
			_ = json.Unmarshal([]byte(platforms), &platformList)
			result = append(result, map[string]any{"id": id, "assignment": assignment, "language": language, "platforms": platformList, "enabled": enabled == 1, "researchIntervalSeconds": interval, "pendingDraftCount": pending, "isRunning": running > 0, "createdAt": created, "updatedAt": updated})
		}
		if err := rows.Err(); err != nil {
			jsonError(w, 500, "database_error", "Could not read agents.")
			return
		}
		jsonResponse(w, 200, map[string]any{"agents": result})
		return
	}
	if len(rest) == 2 && rest[1] == "messages" && r.Method == "GET" {
		a.messages(w, rest[0])
		return
	}
	if len(rest) == 2 && rest[1] == "messages" && r.Method == "POST" {
		notImplemented(w)
		return
	}
	if len(rest) == 2 && rest[1] == "runs" && r.Method == "POST" {
		a.startRun(w, rest[0])
		return
	}
	jsonError(w, 404, "not_found", "Route not found.")
}
func (a *api) startRun(w http.ResponseWriter, agentID string) {
	decodedID, err := url.PathUnescape(agentID)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid_request", "Invalid agent identifier.")
		return
	}
	var exists int
	if err := a.db.QueryRow(`SELECT COUNT(*) FROM agents WHERE id = ?`, decodedID).Scan(&exists); err != nil {
		jsonError(w, http.StatusInternalServerError, "database_error", "Could not find agent.")
		return
	}
	if exists == 0 {
		jsonError(w, http.StatusNotFound, "not_found", "Agent not found.")
		return
	}
	if a.queue == nil {
		jsonError(w, http.StatusServiceUnavailable, "configuration_error", "Research providers are not configured.")
		return
	}
	run, err := a.queue.enqueue(decodedID)
	switch {
	case err == nil:
		jsonResponse(w, http.StatusAccepted, map[string]any{"run": map[string]any{"id": run.ID, "agentId": run.AgentID, "status": run.Status, "startedAt": run.StartedAt, "endedAt": nil, "error": nil}})
	case errors.Is(err, errUnknownAgent):
		jsonError(w, http.StatusNotFound, "not_found", "Agent not found.")
	case errors.Is(err, errActiveRun):
		jsonError(w, http.StatusConflict, "conflict", "This agent already has an active run.")
	case errors.Is(err, errQueueFull):
		jsonError(w, http.StatusServiceUnavailable, "queue_full", "Research queue is full. Try again shortly.")
	case errors.Is(err, errNotConfigured):
		jsonError(w, http.StatusServiceUnavailable, "configuration_error", "Research requires EXA_API_KEY, OPENROUTER_API_KEY, and OPENROUTER_MODEL.")
	default:
		a.log.Error("queue research run", "error", err)
		jsonError(w, http.StatusInternalServerError, "database_error", "Could not create research run.")
	}
}
func (a *api) messages(w http.ResponseWriter, agentID string) {
	decodedID, err := url.PathUnescape(agentID)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid_request", "Invalid agent identifier.")
		return
	}
	var exists int
	if err := a.db.QueryRow(`SELECT COUNT(*) FROM agents WHERE id = ?`, decodedID).Scan(&exists); err != nil {
		jsonError(w, http.StatusInternalServerError, "database_error", "Could not find agent.")
		return
	}
	if exists == 0 {
		jsonError(w, http.StatusNotFound, "not_found", "Agent not found.")
		return
	}

	rows, err := a.db.Query(`SELECT id, role, message_type, text, draft_id, run_id, created_at FROM messages WHERE agent_id=? ORDER BY created_at ASC, id ASC`, decodedID)
	if err != nil {
		jsonError(w, 500, "database_error", "Could not read messages.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, role, typ, text, created string
		var draft, run sql.NullString
		if err := rows.Scan(&id, &role, &typ, &text, &draft, &run, &created); err != nil {
			jsonError(w, 500, "database_error", "Could not read messages.")
			return
		}
		item := map[string]any{"id": id, "agentId": decodedID, "role": role, "messageType": typ, "text": text, "draftId": nil, "runId": nil, "createdAt": created}
		if draft.Valid {
			item["draftId"] = draft.String
		}
		if run.Valid {
			item["runId"] = run.String
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		jsonError(w, 500, "database_error", "Could not read messages.")
		return
	}
	if err := rows.Close(); err != nil {
		jsonError(w, 500, "database_error", "Could not read messages.")
		return
	}
	for _, item := range items {
		draftID, ok := item["draftId"].(string)
		if !ok {
			continue
		}
		draft, err := a.draftPayload(draftID)
		if err != nil {
			jsonError(w, 500, "database_error", "Could not read draft.")
			return
		}
		item["draft"] = draft
	}
	jsonResponse(w, 200, map[string]any{"messages": items})
}

func (a *api) draftPayload(draftID string) (map[string]any, error) {
	var id, agentID, storyID, runID, headline, claimStatus, facebookText, xText, reviewStatus, createdAt, updatedAt string
	err := a.db.QueryRow(`SELECT id, agent_id, story_id, run_id, headline, claim_status, facebook_text, x_text, review_status, created_at, updated_at FROM drafts WHERE id=?`, draftID).Scan(&id, &agentID, &storyID, &runID, &headline, &claimStatus, &facebookText, &xText, &reviewStatus, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	rows, err := a.db.Query(`SELECT id, url, title, published_at, retrieved_at FROM sources WHERE story_id=? ORDER BY id`, storyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sources := []map[string]any{}
	for rows.Next() {
		var sourceID, sourceURL, sourceTitle, retrievedAt string
		var publishedAt sql.NullString
		if err := rows.Scan(&sourceID, &sourceURL, &sourceTitle, &publishedAt, &retrievedAt); err != nil {
			return nil, err
		}
		source := map[string]any{"id": sourceID, "url": sourceURL, "title": sourceTitle, "publishedAt": nil, "retrievedAt": retrievedAt}
		if publishedAt.Valid {
			source["publishedAt"] = publishedAt.String
		}
		sources = append(sources, source)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "agentId": agentID, "storyId": storyID, "runId": runID, "headline": headline, "claimStatus": claimStatus, "facebookText": facebookText, "xText": xText, "reviewStatus": reviewStatus, "sources": sources, "createdAt": createdAt, "updatedAt": updatedAt}, nil
}
