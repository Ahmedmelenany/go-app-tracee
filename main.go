package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "modernc.org/sqlite"
)

type Item struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type store struct {
	db *sql.DB
}

func newStore(path string) (*store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL
	)`); err != nil {
		return nil, err
	}
	return &store{db: db}, nil
}

func (s *store) list() ([]Item, error) {
	rows, err := s.db.Query("SELECT id, name FROM items ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Item{}
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ID, &it.Name); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *store) get(id int64) (Item, error) {
	var it Item
	err := s.db.QueryRow("SELECT id, name FROM items WHERE id = ?", id).Scan(&it.ID, &it.Name)
	return it, err
}

func (s *store) create(name string) (Item, error) {
	res, err := s.db.Exec("INSERT INTO items (name) VALUES (?)", name)
	if err != nil {
		return Item{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Item{}, err
	}
	return Item{ID: id, Name: name}, nil
}

func (s *store) update(id int64, name string) (Item, error) {
	res, err := s.db.Exec("UPDATE items SET name = ? WHERE id = ?", name, id)
	if err != nil {
		return Item{}, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return Item{}, err
	}
	if n == 0 {
		return Item{}, sql.ErrNoRows
	}
	return Item{ID: id, Name: name}, nil
}

func (s *store) delete(id int64) error {
	res, err := s.db.Exec("DELETE FROM items WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func parseID(r *http.Request) (int64, error) {
	var id int64
	_, err := fmt.Sscanf(r.PathValue("id"), "%d", &id)
	return id, err
}

func newRouter(s *store) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /items", func(w http.ResponseWriter, r *http.Request) {
		items, err := s.list()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, items)
	})

	mux.HandleFunc("POST /items", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if body.Name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}
		it, err := s.create(body.Name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, it)
	})

	mux.HandleFunc("GET /items/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}
		it, err := s.get(id)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, it)
	})

	mux.HandleFunc("PUT /items/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}
		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if body.Name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}
		it, err := s.update(id, body.Name)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, it)
	})

	mux.HandleFunc("DELETE /items/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}
		err = s.delete(id)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	return mux
}

func main() {
	s, err := newStore("app.db")
	if err != nil {
		log.Fatalf("store init: %v", err)
	}
	defer s.db.Close()

	srv := &http.Server{
		Addr:    ":8080",
		Handler: newRouter(s),
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
