package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/monitor"
)

type MonitorAPI interface {
	Current() (monitor.Sample, bool)
	History(context.Context, int) ([]monitor.HistoryPoint, error)
}

func registerMonitorRoutes(router interface {
	Get(string, http.HandlerFunc)
}, adminAuth AdminAuthenticator, monitorAPI MonitorAPI) {
	router.Get("/api/v1/admin/monitor/current", func(response http.ResponseWriter, request *http.Request) {
		if _, ok := operationalAdmin(response, request, adminAuth, false); !ok {
			return
		}
		sample, ok := monitorAPI.Current()
		if !ok {
			writeProblem(response, request, http.StatusServiceUnavailable, "MONITOR_WARMING_UP", "Monitor has no sample yet.", true)
			return
		}
		response.Header().Set("Cache-Control", "no-store")
		writeJSON(response, http.StatusOK, sample)
	})

	router.Get("/api/v1/admin/monitor/history", func(response http.ResponseWriter, request *http.Request) {
		if _, ok := operationalAdmin(response, request, adminAuth, false); !ok {
			return
		}
		minutes := monitor.DefaultHistoryMinutes
		if raw := request.URL.Query().Get("minutes"); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil {
				writeProblem(response, request, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Monitor range is invalid.", false)
				return
			}
			minutes = parsed
		}
		points, err := monitorAPI.History(request.Context(), minutes)
		switch {
		case errors.Is(err, monitor.ErrInvalidRange):
			writeProblem(response, request, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Monitor range is invalid.", false)
			return
		case err != nil:
			writeProblem(response, request, http.StatusInternalServerError, "INTERNAL_ERROR", "Monitor history is unavailable.", true)
			return
		}
		response.Header().Set("Cache-Control", "no-store")
		writeJSON(response, http.StatusOK, map[string]any{"minutes": minutes, "data": points})
	})
}
