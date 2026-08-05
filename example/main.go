// Interactive SDK Test Server (run with: go run ./pkg/client/example/main.go)
// Serves an interactive UI at http://localhost:8091 that tests the Go Client SDK.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"

	client "github.com/aruncs31s/goexportclient"
)

type sdkTestConfig struct {
	BaseURL  string `json:"base_url"`
	Token    string `json:"token"`
	TenantID string `json:"tenant_id"`
	UserID   string `json:"user_id"`
}

func getClient(r *http.Request) *client.Client {
	baseURL := r.Header.Get("X-Target-API")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	token := r.Header.Get("Authorization")
	tenantID := r.Header.Get("X-Tenant-ID")
	userID := r.Header.Get("X-User-ID")

	return client.New(
		baseURL, token,
		client.WithTenant(tenantID),
		client.WithUser(userID),
	)
}

func main() {
	mux := http.NewServeMux()

	// Serve test UI HTML
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "pkg/client/example/index.html")
	})

	// Upload HTML temporary helper for testing
	mux.HandleFunc("POST /uploads", func(w http.ResponseWriter, r *http.Request) {
		r.ParseMultipartForm(2 << 20)
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "file required", 400)
			return
		}
		defer file.Close()
		body, _ := io.ReadAll(file)
		name := filepath.Base(header.Filename)
		_ = os.WriteFile("/tmp/"+name, body, 0o644)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"url": "file:///tmp/" + name})
	})

	// 1. High-level ExportURL helper via SDK
	mux.HandleFunc("POST /sdk/export-url", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			URL     string `json:"url"`
			Section string `json:"section"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		c := getClient(r)
		pdfBytes, err := c.ExportURL(r.Context(), body.URL, body.Section, 0)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", `inline; filename="sdk-export.pdf"`)
		w.Write(pdfBytes)
	})

	// 1.1 High-level ExportHTML helper via SDK
	mux.HandleFunc("POST /sdk/export-html", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			HTML    string `json:"html"`
			Section string `json:"section"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		c := getClient(r)
		pdfBytes, err := c.ExportHTML(r.Context(), body.HTML, body.Section, 0)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", `inline; filename="sdk-export.pdf"`)
		w.Write(pdfBytes)
	})

	// 2. CreateExport via SDK
	mux.HandleFunc("POST /sdk/create", func(w http.ResponseWriter, r *http.Request) {
		var req client.ExportRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		c := getClient(r)
		resp, err := c.CreateExport(r.Context(), req)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	// 3. GetStatus via SDK
	mux.HandleFunc("GET /sdk/status", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		c := getClient(r)
		status, err := c.GetStatus(r.Context(), id)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	})

	// 4. DownloadPDF via SDK
	mux.HandleFunc("GET /sdk/download", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		c := getClient(r)
		pdfBytes, err := c.DownloadPDF(r.Context(), id)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", mime.TypeByExtension(".pdf"))
		w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s.pdf"`, id))
		w.Write(pdfBytes)
	})

	// 5. ListMyExports via SDK
	mux.HandleFunc("GET /sdk/list-my", func(w http.ResponseWriter, r *http.Request) {
		section := r.URL.Query().Get("section")
		c := getClient(r)
		list, err := c.ListMyExports(r.Context(), section, 20, 0)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(list)
	})

	log.Println("Go SDK interactive test server listening on http://localhost:8091")
	log.Fatal(http.ListenAndServe(":8091", mux))
}
