package main

import (
	"archive/zip"
	"encoding/json"
	"encoding/xml"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ── Structures EPUB ───────────────────────────────────────────────────────────

type Container struct {
	Rootfiles []struct {
		Rootfile []struct {
			FullPath string `xml:"full-path,attr"`
		} `xml:"rootfile"`
	} `xml:"rootfiles"`
}

type OPFMetadata struct {
	Title       []string `xml:"title"`
	Creator     []string `xml:"creator"`
	Language    []string `xml:"language"`
	Publisher   []string `xml:"publisher"`
	Description []string `xml:"description"`
	Subject     []string `xml:"subject"`
	Date        []string `xml:"date"`
}

type OPFPackage struct {
	Metadata OPFMetadata `xml:"metadata"`
}

// ── Structures JSON ───────────────────────────────────────────────────────────

type BookMetadata struct {
	Title       *string  `json:"title"`
	Authors     []string `json:"authors,omitempty"`
	Language    *string  `json:"language,omitempty"`
	Publisher   *string  `json:"publisher,omitempty"`
	Description *string  `json:"description,omitempty"`
	Subjects    []string `json:"subjects,omitempty"`
	Date        *string  `json:"date,omitempty"`
}

type Book struct {
	Name         string        `json:"name"`
	DisplayName  string        `json:"displayName"`
	Path         string        `json:"path"`
	Size         int64         `json:"size"`
	ModifiedDate string        `json:"modifiedDate"`
	Metadata     *BookMetadata `json:"metadata"`
}

type BooksResponse struct {
	Count int    `json:"count"`
	Files []Book `json:"files"`
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func first(ss []string) *string {
	if len(ss) == 0 {
		return nil
	}
	return strPtr(strings.TrimSpace(ss[0]))
}

// ── Extraction des métadonnées EPUB ──────────────────────────────────────────

func extractMetadata(filePath string) (*BookMetadata, error) {
	r, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	var opfPath string
	for _, f := range r.File {
		if f.Name == "META-INF/container.xml" {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			data, _ := io.ReadAll(rc)
			rc.Close()

			var c Container
			if err := xml.Unmarshal(data, &c); err != nil {
				return nil, err
			}
			if len(c.Rootfiles) > 0 && len(c.Rootfiles[0].Rootfile) > 0 {
				opfPath = c.Rootfiles[0].Rootfile[0].FullPath
			}
			break
		}
	}

	if opfPath == "" {
		return nil, nil
	}

	for _, f := range r.File {
		if f.Name == opfPath {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			data, _ := io.ReadAll(rc)
			rc.Close()

			var pkg OPFPackage
			if err := xml.Unmarshal(data, &pkg); err != nil {
				return nil, err
			}

			m := pkg.Metadata

			authors := make([]string, 0, len(m.Creator))
			for _, a := range m.Creator {
				if s := strings.TrimSpace(a); s != "" {
					authors = append(authors, s)
				}
			}
			subjects := make([]string, 0, len(m.Subject))
			for _, s := range m.Subject {
				if s := strings.TrimSpace(s); s != "" {
					subjects = append(subjects, s)
				}
			}

			meta := &BookMetadata{
				Title:       first(m.Title),
				Authors:     authors,
				Language:    first(m.Language),
				Publisher:   first(m.Publisher),
				Description: first(m.Description),
				Subjects:    subjects,
				Date:        first(m.Date),
			}
			return meta, nil
		}
	}

	return nil, nil
}

// ── Auth middleware ───────────────────────────────────────────────────────────

// authMiddleware vérifie le Bearer token auprès de auth-service.
// Les requêtes sans token valide reçoivent un 401.
func authMiddleware(authURL string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, authURL+"/auth/me", nil)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		req.Header.Set("Authorization", "Bearer "+token)

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		resp.Body.Close()

		next.ServeHTTP(w, r)
	})
}

// ── Handler API ───────────────────────────────────────────────────────────────

func booksHandler(ressourcesPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		entries, err := os.ReadDir(ressourcesPath)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(BooksResponse{Count: 0, Files: []Book{}})
			return
		}

		books := make([]Book, 0)

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if strings.ToLower(filepath.Ext(name)) != ".epub" {
				continue
			}

			fullPath := filepath.Join(ressourcesPath, name)
			info, err := entry.Info()
			if err != nil {
				continue
			}

			meta, _ := extractMetadata(fullPath)

			displayName := strings.TrimSuffix(name, filepath.Ext(name))
			if meta != nil && meta.Title != nil {
				displayName = *meta.Title
			}

			books = append(books, Book{
				Name:         name,
				DisplayName:  displayName,
				Path:         "/ressources/" + name,
				Size:         info.Size(),
				ModifiedDate: info.ModTime().UTC().Format(time.RFC3339),
				Metadata:     meta,
			})
		}

		sort.Slice(books, func(i, j int) bool {
			return strings.ToLower(books[i].DisplayName) < strings.ToLower(books[j].DisplayName)
		})

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-cache")
		json.NewEncoder(w).Encode(BooksResponse{Count: len(books), Files: books})
	}
}

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	ressourcesPath := os.Getenv("RESSOURCES_PATH")
	if ressourcesPath == "" {
		ressourcesPath = "./ressources"
	}

	// URL du service auth (ex: https://auth.vivalink.top)
	authURL := os.Getenv("AUTH_URL")
	if authURL == "" {
		log.Fatal("AUTH_URL environment variable is required")
	}

	mux := http.NewServeMux()

	// Routes protégées par auth
	protected := func(h http.Handler) http.Handler {
		return authMiddleware(authURL, h)
	}

	// API — protégée
	mux.Handle("/api/books", protected(booksHandler(ressourcesPath)))

	// Téléchargement EPUBs — protégé
	fs := http.FileServer(http.Dir(ressourcesPath))
	mux.Handle("/ressources/", protected(http.StripPrefix("/ressources/", fs)))

	// Frontend statique — public (gère la page de login)
	mux.Handle("/", http.FileServer(http.Dir("./public")))

	log.Printf("📚 Library server on :%s", port)
	log.Printf("📂 Ressources: %s", ressourcesPath)
	log.Printf("🔐 Auth: %s", authURL)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
