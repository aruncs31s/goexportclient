# GoExport Client

[![Go Reference](https://pkg.go.dev/badge/github.com/aruncs31s/goexportclient.svg)](https://pkg.go.dev/github.com/aruncs31s/goexportclient)

Official Go SDK driver for the **GoExport** SaaS — a headless Chrome PDF generation service. Connect to any hosted GoExport instance with just a URL and a JWT token.

```go
c := goexporclient.New("https://goexport.onrender.com", "your-jwt-token",
    goexporclient.WithTenant("inst-101"),
    goexporclient.WithUser("user-42"),
)

// One call: submit → poll → download PDF bytes
pdfBytes, err := c.ExportURL(ctx, "https://myapp.com/invoice/42", "invoices", 0)
```

---

## Table of Contents

- [Requirements](#requirements)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Authentication](#authentication)
- [Multi-Tenancy](#multi-tenancy)
- [API Reference](#api-reference)
  - [Constructor](#constructor)
  - [High-Level Helpers](#high-level-helpers)
  - [Low-Level Methods](#low-level-methods)
  - [Types](#types)
- [Error Handling](#error-handling)
- [Configuration Options](#configuration-options)
- [Per-Call Overrides](#per-call-overrides)
- [Example: Sections & Sections](#example-full-workflow)
- [Interactive Test Console](#interactive-test-console)

---

## Requirements

- **Go 1.21+**
- A running [GoExport](https://github.com/aruncs31s/goexport) server instance
- A valid **JWT access token** (generated from the GoExport admin dashboard at `/admin/tokens`)

---

## Installation

```bash
go get github.com/aruncs31s/goexportclient
```

---

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "os"

    goexporclient "github.com/aruncs31s/goexportclient"
)

func main() {
    c := goexporclient.New(
        "https://goexport.onrender.com",
        os.Getenv("GOEXPORT_TOKEN"),
        goexporclient.WithTenant("inst-101"),
        goexporclient.WithUser("user-42"),
    )

    ctx := context.Background()

    // --- Render a URL to PDF ---
    pdfBytes, err := c.ExportURL(ctx, "https://example.com/invoice/42", "invoices", 0)
    if err != nil {
        panic(err)
    }
    os.WriteFile("invoice.pdf", pdfBytes, 0644)
    fmt.Println("PDF saved!")

    // --- Render raw HTML to PDF ---
    html := `<html><body><h1>Receipt</h1><p>Total: $100</p></body></html>`
    pdfBytes, err = c.ExportHTML(ctx, html, "receipts", 0)
    if err != nil {
        panic(err)
    }
    os.WriteFile("receipt.pdf", pdfBytes, 0644)
    fmt.Println("HTML PDF saved!")
}
```

---

## Authentication

GoExport uses **JWT Bearer tokens**. Tokens are created in the admin dashboard (`/admin/tokens`) and are scoped to a named application, with a configurable expiry.

Pass the token to `New()`:

```go
c := goexporclient.New("https://goexport.example.com", "eyJhbGci...")
```

The SDK automatically sets the `Authorization: Bearer <token>` header on every request.

---

## Multi-Tenancy

GoExport supports multi-tenant isolation. Each request can carry a **Tenant ID** and **User ID**, which are used to scope export history and access control.

**Set defaults at client initialization:**

```go
c := goexporclient.New(baseURL, token,
    goexporclient.WithTenant("school-101"),
    goexporclient.WithUser("teacher-99"),
)
```

**Override per individual call:**

```go
pdfBytes, err := c.ExportURL(ctx, url, "reports", 0,
    goexporclient.WithCallTenant("school-202"),
    goexporclient.WithCallUser("admin-1"),
)
```

These map to the `X-Tenant-ID` and `X-User-ID` HTTP headers.

---

## API Reference

### Constructor

#### `New(baseURL, token string, opts ...Option) *Client`

Creates and returns a new GoExport client driver.

| Parameter | Type | Description |
|-----------|------|-------------|
| `baseURL` | `string` | Base URL of the GoExport instance, e.g. `https://goexport.example.com` |
| `token` | `string` | JWT Bearer token generated from the admin dashboard |
| `opts` | `...Option` | Optional configuration (see [Configuration Options](#configuration-options)) |

---

### High-Level Helpers

These are the recommended entry points. They submit a job, poll until complete, and return the PDF bytes in a single call.

#### `ExportURL(ctx, targetURL, section string, pollInterval time.Duration, opts ...CallOption) ([]byte, error)`

Renders a publicly accessible URL as a PDF.

```go
pdfBytes, err := c.ExportURL(ctx, "https://myapp.com/invoice/42", "invoices", 0)
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `targetURL` | `string` | The URL to render. Must be `http://` or `https://` |
| `section` | `string` | Logical grouping for the export (e.g. `"invoices"`, `"reports"`) |
| `pollInterval` | `time.Duration` | How often to poll status. `0` uses the default of 2 seconds |
| `opts` | `...CallOption` | Per-call tenant/user overrides |

---

#### `ExportHTML(ctx, html, section string, pollInterval time.Duration, opts ...CallOption) ([]byte, error)`

Renders raw HTML directly as a PDF — no URL required. Chrome receives the HTML payload via `SetDocumentContent`, bypassing any network access.

```go
html := `<html><body><h1>My Report</h1></body></html>`
pdfBytes, err := c.ExportHTML(ctx, html, "reports", 0)
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `html` | `string` | Full HTML content to render |
| `section` | `string` | Logical grouping for the export |
| `pollInterval` | `time.Duration` | How often to poll. `0` = 2 second default |
| `opts` | `...CallOption` | Per-call overrides |

> **Note:** `url` and `html` are mutually exclusive. Passing both will return a `400 Bad Request` from the server.

---

### Low-Level Methods

Use these when you need fine-grained control over the job lifecycle.

#### `CreateExport(ctx, req ExportRequest, opts ...CallOption) (*ExportResponse, error)`

Submits a new PDF export job to the queue. Returns immediately with a job ID.

```go
resp, err := c.CreateExport(ctx, goexporclient.ExportRequest{
    URL:         "https://myapp.com/invoice/1",
    Section:     "invoices",
    CallbackURL: "https://myapp.com/webhooks/pdf-done",
})
fmt.Println("Job queued:", resp.ID)
```

---

#### `GetStatus(ctx, id string, opts ...CallOption) (*ExportStatus, error)`

Polls the status of an export job by its ID.

```go
status, err := c.GetStatus(ctx, jobID)
fmt.Println(status.State) // "queued" | "processing" | "completed" | "failed"
```

---

#### `DownloadPDF(ctx, id string, opts ...CallOption) ([]byte, error)`

Downloads the rendered PDF bytes for a `completed` job.

```go
pdfBytes, err := c.DownloadPDF(ctx, jobID)
os.WriteFile("output.pdf", pdfBytes, 0644)
```

---

#### `ListMyExports(ctx, section string, limit, offset int, opts ...CallOption) (*ExportListResponse, error)`

Lists export records scoped to the requesting user and tenant.

```go
list, err := c.ListMyExports(ctx, "invoices", 20, 0)
for _, e := range list.Exports {
    fmt.Printf("[%s] %s — %s\n", e.ID[:8], e.URL, e.State)
}
```

---

### Types

#### `ExportRequest`

```go
type ExportRequest struct {
    URL         string `json:"url"`              // Mutually exclusive with HTML
    HTML        string `json:"html,omitempty"`   // Mutually exclusive with URL
    Section     string `json:"section,omitempty"`
    CallbackURL string `json:"callback_url,omitempty"` // Optional webhook on completion
}
```

#### `ExportResponse`

```go
type ExportResponse struct {
    ID      string `json:"id"`
    Section string `json:"section"`
    State   string `json:"state"` // Always "queued" on creation
}
```

#### `ExportStatus`

```go
type ExportStatus struct {
    ID          string    `json:"id"`
    URL         string    `json:"url"`
    Section     string    `json:"section"`
    TenantID    string    `json:"tenant_id,omitempty"`
    UserID      string    `json:"user_id,omitempty"`
    State       string    `json:"state"`           // queued | processing | completed | failed
    ObjectKey   string    `json:"object_key,omitempty"`
    Error       string    `json:"error,omitempty"`
    CallbackURL string    `json:"callback_url,omitempty"`
    CreatedAt   time.Time `json:"created_at"`
    CompletedAt time.Time `json:"completed_at,omitempty"`
}
```

#### `ExportListResponse`

```go
type ExportListResponse struct {
    Exports []ExportStatus `json:"exports"`
    Count   int            `json:"count"`
    Total   int            `json:"total"`
    Limit   int            `json:"limit"`
    Offset  int            `json:"offset"`
}
```

---

## Error Handling

All methods return a descriptive `error`. API errors include the HTTP status code and message:

```go
pdfBytes, err := c.ExportURL(ctx, url, "invoices", 0)
if err != nil {
    // e.g. "API error (401): not authenticated"
    //      "API error (400): url or html is required"
    //      "export job abc123: render failed: ..."
    log.Fatal(err)
}
```

You can also catch context cancellation:

```go
ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
defer cancel()

pdfBytes, err := c.ExportURL(ctx, url, "invoices", 0)
if err != nil {
    if errors.Is(err, context.DeadlineExceeded) {
        log.Println("PDF generation timed out")
    }
}
```

---

## Configuration Options

Passed to `New()` to configure the client globally:

| Option | Description |
|--------|-------------|
| `WithTenant(tenantID string)` | Sets the default `X-Tenant-ID` header |
| `WithUser(userID string)` | Sets the default `X-User-ID` header |
| `WithHTTPClient(*http.Client)` | Replaces the default HTTP client (useful for custom timeouts or transport) |

```go
c := goexporclient.New(baseURL, token,
    goexporclient.WithTenant("school-101"),
    goexporclient.WithUser("teacher-99"),
    goexporclient.WithHTTPClient(&http.Client{Timeout: 60 * time.Second}),
)
```

---

## Per-Call Overrides

Passed to individual SDK method calls to override the client-level tenant/user for that request only:

| Option | Description |
|--------|-------------|
| `WithCallTenant(tenantID string)` | Overrides `X-Tenant-ID` for this call |
| `WithCallUser(userID string)` | Overrides `X-User-ID` for this call |

```go
// Use a different tenant for one specific call
pdfBytes, err := c.ExportURL(ctx, url, "reports", 0,
    goexporclient.WithCallTenant("partner-org"),
    goexporclient.WithCallUser("user-x"),
)
```

---

## Example: Full Workflow

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"
    "time"

    goexporclient "github.com/aruncs31s/goexportclient"
)

func main() {
    c := goexporclient.New(
        "https://goexport.onrender.com",
        os.Getenv("GOEXPORT_TOKEN"),
        goexporclient.WithTenant("school-101"),
        goexporclient.WithUser("staff-42"),
    )

    ctx := context.Background()

    // --- Step 1: Submit job manually ---
    resp, err := c.CreateExport(ctx, goexporclient.ExportRequest{
        URL:         "https://myapp.com/reports/monthly",
        Section:     "reports",
        CallbackURL: "https://myapp.com/webhooks/pdf",
    })
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println("Job created:", resp.ID)

    // --- Step 2: Poll manually ---
    for {
        status, err := c.GetStatus(ctx, resp.ID)
        if err != nil {
            log.Fatal(err)
        }
        fmt.Println("Status:", status.State)
        if status.State == "completed" {
            break
        }
        if status.State == "failed" {
            log.Fatal("Job failed:", status.Error)
        }
        time.Sleep(2 * time.Second)
    }

    // --- Step 3: Download ---
    pdfBytes, err := c.DownloadPDF(ctx, resp.ID)
    if err != nil {
        log.Fatal(err)
    }
    os.WriteFile("monthly-report.pdf", pdfBytes, 0644)
    fmt.Printf("PDF saved (%d bytes)\n", len(pdfBytes))

    // --- Or, do it all in one call ---
    pdfBytes, err = c.ExportURL(ctx, "https://myapp.com/invoice/999", "invoices", 2*time.Second)
    if err != nil {
        log.Fatal(err)
    }
    os.WriteFile("invoice.pdf", pdfBytes, 0644)
    fmt.Println("Invoice saved!")
}
```

---

## Interactive Test Console

The `example/` directory contains a standalone interactive console server for testing the SDK in a browser.

```bash
# From the repo root:
go run ./example/main.go
```

Open **http://localhost:8091** in your browser. The console lets you:

- Configure the target GoExport server URL and JWT token
- Set Tenant ID and User ID headers
- Test all SDK methods interactively:
  - `client.ExportURL()` — render a URL to PDF
  - `client.ExportHTML()` — render raw HTML to PDF
  - `client.CreateExport()` — queue a job manually
  - `client.GetStatus()` — poll a job by ID
  - `client.DownloadPDF()` — download the result
  - `client.ListMyExports()` — list your export history
