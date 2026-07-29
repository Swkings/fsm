package visualization

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) handleReplaySources(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.replays.views())
	case http.MethodPost:
		var c ReplaySourceConfig
		if e := decodeJSONBody(w, r, &c); e != nil {
			writeJSON(w, 400, map[string]string{"error": e.Error()})
			return
		}
		v, e := s.replays.add(r.Context(), c)
		if e != nil {
			writeJSON(w, 409, map[string]string{"error": e.Error()})
			return
		}
		writeJSON(w, 201, v)
	default:
		w.WriteHeader(405)
	}
}
func (s *Server) handleReplaySourceDetail(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/replay/sources/"), "/")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodDelete {
		w.WriteHeader(405)
		return
	}
	if !s.replays.drop(id) {
		writeJSON(w, 404, map[string]string{"error": ErrReplaySourceNotFound.Error()})
		return
	}
	w.WriteHeader(204)
}
func (s *Server) handleReplayRecords(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(405)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	records, e := s.replays.list(r.Context(), r.URL.Query().Get("source"), r.URL.Query().Get("fsmId"), limit)
	if e != nil {
		writeJSON(w, 502, map[string]string{"error": e.Error()})
		return
	}
	writeJSON(w, 200, records)
}
func (s *Server) handleReplayRecordDetail(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/replay/records/"), "/")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	source := r.URL.Query().Get("source")
	switch r.Method {
	case http.MethodGet:
		record, e := s.replays.get(r.Context(), source, id)
		if e != nil {
			status := 502
			if errors.Is(e, ErrReplayRecordNotFound) {
				status = 404
			}
			writeJSON(w, status, map[string]string{"error": e.Error()})
			return
		}
		writeJSON(w, 200, record)
	case http.MethodDelete:
		e := s.replays.remove(r.Context(), source, id)
		if e != nil {
			status := 409
			if errors.Is(e, ErrReplayRecordNotFound) {
				status = 404
			}
			writeJSON(w, status, map[string]string{"error": e.Error()})
			return
		}
		w.WriteHeader(204)
	default:
		w.WriteHeader(405)
	}
}
