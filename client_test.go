package goexporclient_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	client "github.com/aruncs31s/goexportclient"
)

func TestClient_CreateExportAndDownloadPDF(t *testing.T) {
	var receivedAuth, receivedTenant, receivedUser string
	pollCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		receivedTenant = r.Header.Get("X-Tenant-ID")
		receivedUser = r.Header.Get("X-User-ID")

		switch r.URL.Path {
		case "/exports":
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s, want POST", r.Method)
			}
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(client.ExportResponse{
				ID:      "job-123",
				Section: "invoices",
				State:   "queued",
			})
		case "/exports/job-123":
			pollCount++
			w.WriteHeader(http.StatusOK)
			state := "processing"
			if pollCount >= 2 {
				state = "completed"
			}
			_ = json.NewEncoder(w).Encode(client.ExportStatus{
				ID:        "job-123",
				URL:       "https://example.com/invoice",
				Section:   "invoices",
				TenantID:  "inst-101",
				UserID:    "user-42",
				State:     state,
				ObjectKey: "exports/inst-101/invoices/job-123.pdf",
			})
		case "/exports/job-123/pdf":
			w.Header().Set("Content-Type", "application/pdf")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("mock-pdf-content"))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	c := client.New(
		server.URL, "secret-token",
		client.WithTenant("inst-101"),
		client.WithUser("user-42"),
	)

	pdfBytes, err := c.ExportURL(context.Background(), "https://example.com/invoice", "invoices", 10*time.Millisecond)
	if err != nil {
		t.Fatalf("ExportURL error = %v", err)
	}

	if string(pdfBytes) != "mock-pdf-content" {
		t.Fatalf("pdfBytes = %q, want 'mock-pdf-content'", string(pdfBytes))
	}
	if receivedAuth != "Bearer secret-token" {
		t.Fatalf("auth header = %q, want 'Bearer secret-token'", receivedAuth)
	}
	if receivedTenant != "inst-101" {
		t.Fatalf("tenant header = %q, want 'inst-101'", receivedTenant)
	}
	if receivedUser != "user-42" {
		t.Fatalf("user header = %q, want 'user-42'", receivedUser)
	}
}

func TestClient_ListMyExports(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/exports/my" {
			t.Fatalf("path = %s, want /exports/my", r.URL.Path)
		}
		if r.URL.Query().Get("section") != "academics" {
			t.Fatalf("section = %s, want academics", r.URL.Query().Get("section"))
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(client.ExportListResponse{
			Exports: []client.ExportStatus{{ID: "e-1", Section: "academics"}},
			Count:   1,
			Total:   1,
		})
	}))
	defer server.Close()

	c := client.New(server.URL, "token")
	list, err := c.ListMyExports(context.Background(), "academics", 10, 0, client.WithCallTenant("tenant-a"), client.WithCallUser("user-b"))
	if err != nil {
		t.Fatalf("ListMyExports error = %v", err)
	}
	if list.Count != 1 || len(list.Exports) != 1 || list.Exports[0].ID != "e-1" {
		t.Fatalf("list = %+v", list)
	}
}

func TestClient_ExportHTML(t *testing.T) {
	var receivedHTML string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/exports":
			var req struct {
				URL     string `json:"url"`
				HTML    string `json:"html"`
				Section string `json:"section"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			receivedHTML = req.HTML
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(client.ExportResponse{
				ID:      "job-html",
				Section: "reports",
				State:   "queued",
			})
		case "/exports/job-html":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(client.ExportStatus{
				ID:      "job-html",
				Section: "reports",
				State:   "completed",
			})
		case "/exports/job-html/pdf":
			w.Header().Set("Content-Type", "application/pdf")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("mock-html-pdf-content"))
		}
	}))
	defer server.Close()

	c := client.New(server.URL, "token")
	pdfBytes, err := c.ExportHTML(context.Background(), "<h1>Hello</h1>", "reports", 10*time.Millisecond)
	if err != nil {
		t.Fatalf("ExportHTML error = %v", err)
	}

	if string(pdfBytes) != "mock-html-pdf-content" {
		t.Fatalf("pdfBytes = %q, want 'mock-html-pdf-content'", string(pdfBytes))
	}
	if receivedHTML != "<h1>Hello</h1>" {
		t.Fatalf("receivedHTML = %q, want '<h1>Hello</h1>'", receivedHTML)
	}
}

func TestClient_CustomFolderOption(t *testing.T) {
	var receivedCustomFolder string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			CustomFolder string `json:"custom_folder"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		receivedCustomFolder = req.CustomFolder
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(client.ExportResponse{
			ID:    "job-folder",
			State: "queued",
		})
	}))
	defer server.Close()

	// 1. Client-level WithCustomFolder
	c := client.New(server.URL, "token", client.WithCustomFolder("global-folder"))
	_, err := c.CreateExport(context.Background(), client.ExportRequest{
		URL:     "https://example.com",
		Section: "reports",
	})
	if err != nil {
		t.Fatalf("CreateExport error = %v", err)
	}
	if receivedCustomFolder != "global-folder" {
		t.Fatalf("receivedCustomFolder = %q, want 'global-folder'", receivedCustomFolder)
	}

	// 2. Per-request WithCallCustomFolder override
	_, err = c.CreateExport(context.Background(), client.ExportRequest{
		URL:     "https://example.com",
		Section: "reports",
	}, client.WithCallCustomFolder("override-folder"))
	if err != nil {
		t.Fatalf("CreateExport error = %v", err)
	}
	if receivedCustomFolder != "override-folder" {
		t.Fatalf("receivedCustomFolder = %q, want 'override-folder'", receivedCustomFolder)
	}

	// 3. Per-request WithS3Storage with optional folder
	_, err = c.CreateExport(context.Background(), client.ExportRequest{
		URL:     "https://example.com",
		Section: "reports",
	}, client.WithS3Storage("http://s3.local", "my-bucket", "ak", "sk", "us-east-1", "s3-folder"))
	if err != nil {
		t.Fatalf("CreateExport error = %v", err)
	}
	if receivedCustomFolder != "s3-folder" {
		t.Fatalf("receivedCustomFolder = %q, want 's3-folder'", receivedCustomFolder)
	}
}
