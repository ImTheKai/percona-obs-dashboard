package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/percona/obs-dashboard/internal/store"
)

// packagesHandler returns a handler for GET /api/products/{product}/{version}/packages.
func packagesHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		product := chi.URLParam(r, "product")
		version := chi.URLParam(r, "version")
		prefix := "isv:percona:" + product + ":" + version

		pkgs, err := store.QueryPackages(db, prefix)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(pkgs); err != nil {
			// Response already started; nothing we can do.
			return
		}
	}
}

// eventsHandler returns a handler for GET /api/products/{product}/{version}/events.
// Query params:
//   - window=<minutes>  — last N minutes (overrides from/to)
//   - from=YYYY-MM-DD   — start of date range (inclusive)
//   - to=YYYY-MM-DD     — end of date range (inclusive, treated as end-of-day)
//
// Default (no params): last 24 hours.
func eventsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		product := chi.URLParam(r, "product")
		version := chi.URLParam(r, "version")
		prefix := "isv:percona:" + product + ":" + version

		now := time.Now().UTC()
		var from, to time.Time

		if windowStr := r.URL.Query().Get("window"); windowStr != "" {
			windowMinutes, err := strconv.Atoi(windowStr)
			if err != nil || windowMinutes <= 0 {
				http.Error(w, "invalid window parameter", http.StatusBadRequest)
				return
			}
			from = now.Add(-time.Duration(windowMinutes) * time.Minute)
			to = now
		} else if fromStr := r.URL.Query().Get("from"); fromStr != "" {
			toStr := r.URL.Query().Get("to")
			if toStr == "" {
				http.Error(w, "to parameter is required when from is set", http.StatusBadRequest)
				return
			}
			const dateLayout = "2006-01-02"
			parsedFrom, err := time.Parse(dateLayout, fromStr)
			if err != nil {
				http.Error(w, "invalid from date", http.StatusBadRequest)
				return
			}
			parsedTo, err := time.Parse(dateLayout, toStr)
			if err != nil {
				http.Error(w, "invalid to date", http.StatusBadRequest)
				return
			}
			from = parsedFrom.UTC()
			// Include all events up to end of the 'to' day.
			to = parsedTo.UTC().Add(24*time.Hour - time.Nanosecond)
		} else {
			// Default: last 24 hours.
			from = now.Add(-24 * time.Hour)
			to = now
		}

		events, err := store.QueryEvents(db, prefix, from, to)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(events); err != nil {
			return
		}
	}
}
