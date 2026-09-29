package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const maxUpload = 200 << 20 // 200 MiB
const converter = "/opt/wav2wem/wav2wem.exe"
const wineHome = "/home/appuser"
const winePrefix = "/home/appuser/.wine"

func main() {
	log.Printf("wav-to-wem API V13 Extended starting")
	log.Printf("converter: Windows wav2wem.exe v0.1 via Wine")
	log.Printf("converter path: %s", converter)
	log.Printf("HOME=%s WINEPREFIX=%s", wineHome, winePrefix)

	if out, err := runWine("--version"); err != nil {
		log.Fatalf("WINE SELF-TEST FAILED: %v: %s", err, clean(out))
	} else {
		log.Printf("WINE SELF-TEST OK: %s", clean(out))
	}

	if out, err := runWine("wineboot", "--init"); err != nil {
		log.Fatalf("WINE PREFIX INIT FAILED: %v: %s", err, clean(out))
	} else {
		log.Printf("WINE PREFIX INIT OK")
	}

	if !fileExists(converter) {
		log.Fatalf("converter missing: %s", converter)
	}

	// This is the important runtime test: it executes the actual Windows
	// wav2wem.exe, so missing bcryptprimitives.dll / ProcessPrng support
	// is caught at startup instead of during a user conversion.
	if out, err := runWine(converter, "--help"); err != nil {
		log.Fatalf("WAV2WEM SELF-TEST FAILED: %v: %s", err, clean(out))
	} else {
		log.Printf("WAV2WEM SELF-TEST OK: Windows binary starts under Wine")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", root)
	mux.HandleFunc("/health", health)
	mux.HandleFunc("/diagnostics", diagnostics)
	mux.HandleFunc("/convert", convert)
	mux.HandleFunc("/youtube/search", youtubeSearch)
	mux.HandleFunc("/youtube/video", youtubeVideo)

	port := os.Getenv("PORT")
	if port == "" {
		port = "10000"
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           cors(mux),
		ReadHeaderTimeout: 15 * time.Second,
	}
	log.Printf("wav-to-wem API listening on :%s", port)
	log.Fatal(srv.ListenAndServe())
}

func root(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"service": "wav-to-wem-converter",
		"ok":      true,
		"version": "v13-extended",
	})
}

func health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": "v13-extended"})
}

func diagnostics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	result := map[string]any{
		"version":           "v13-extended",
		"wine_home":         wineHome,
		"wine_prefix":       winePrefix,
		"converter":         converter,
		"converter_present": fileExists(converter),
		"prefix_present":    fileExists(winePrefix),
	}

	wineOut, wineErr := runWine("--version")
	result["wine_available"] = wineErr == nil
	result["wine_output"] = clean(wineOut)
	if wineErr != nil {
		result["wine_error"] = wineErr.Error()
		writeJSON(w, http.StatusServiceUnavailable, result)
		return
	}

	out, err := runWine(converter, "--help")
	result["wav2wem_self_test"] = err == nil
	result["wav2wem_output"] = clean(out)
	if err != nil {
		result["wav2wem_error"] = err.Error()
		writeJSON(w, http.StatusServiceUnavailable, result)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func convert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !fileExists(converter) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":  "converter is missing",
			"detail": converter,
		})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUpload+1<<20)

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "missing multipart field 'file'",
		})
		return
	}
	defer file.Close()

	if header.Size > maxUpload {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{
			"error": "file is larger than 200 MB",
		})
		return
	}
	if !strings.EqualFold(filepath.Ext(header.Filename), ".wav") {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "only .wav files are accepted",
		})
		return
	}

	dir, err := os.MkdirTemp("", "wav2wem-api-")
	if err != nil {
		http.Error(w, "failed to create temp directory", http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(dir)

	input := filepath.Join(dir, "input.wav")
	output := filepath.Join(dir, "output.wem")

	if err := saveUpload(file, input); err != nil {
		http.Error(w, "failed to save upload", http.StatusInternalServerError)
		return
	}

	args := []string{
		converter,
		winePath(input),
		"-o",
		winePath(output),
		"-q",
		"4",
	}

	log.Printf("starting conversion: %s", header.Filename)
	log.Printf("command: wine %s", strings.Join(args[1:], " "))
	log.Printf("conversion environment: HOME=%s WINEPREFIX=%s", wineHome, winePrefix)

	start := time.Now()
	cmd := exec.Command("wine", args...)
	cmd.Env = wineEnv()
	out, err := cmd.CombinedOutput()
	elapsed := time.Since(start)

	if cleanOut := clean(out); cleanOut != "" {
		log.Printf("wav2wem output: %s", cleanOut)
	}

	if err != nil {
		log.Printf("wav2wem failed after %s: %v", elapsed.Round(time.Millisecond), err)
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
			"error":  "WAV to WEM conversion failed",
			"detail": clean(out),
		})
		return
	}

	log.Printf("conversion succeeded in %s", elapsed.Round(time.Millisecond))

	info, err := os.Stat(output)
	if err != nil || info.Size() == 0 {
		detail := "converter completed but no WEM was produced"
		if err != nil {
			detail += ": " + err.Error()
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":  "WEM output missing",
			"detail": detail,
		})
		return
	}

	data, err := os.Open(output)
	if err != nil {
		http.Error(w, "failed to open WEM output", http.StatusInternalServerError)
		return
	}
	defer data.Close()

	base := strings.TrimSuffix(filepath.Base(header.Filename), filepath.Ext(header.Filename))
	downloadName := sanitizeFilename(base + ".wem")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, downloadName))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, data)
}

// youtubeSearch uses the official YouTube Data API when a key is configured.
// Without a key, it returns the official YouTube search URL instead of failing.
// V13 never scrapes YouTube.
func youtubeSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing search query"})
		return
	}
	if len(q) > 100 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "search query is too long"})
		return
	}

	apiKey := strings.TrimSpace(os.Getenv("YOUTUBE_API_KEY"))
	if apiKey == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"configured": false,
			"query":      q,
			"searchUrl":  "https://www.youtube.com/results?search_query=" + url.QueryEscape(q),
			"message":    "Integrated search is optional. Open YouTube Search, choose a video, and paste its link into the link tab.",
		})
		return
	}

	params := url.Values{}
	params.Set("part", "snippet")
	params.Set("type", "video")
	params.Set("maxResults", "8")
	params.Set("q", q)
	params.Set("key", apiKey)

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet,
		"https://www.googleapis.com/youtube/v3/search?"+params.Encode(), nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create YouTube request"})
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not reach YouTube"})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not read YouTube response"})
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("YouTube search API error: %s", clean(body))
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error":  "YouTube search failed",
			"detail": clean(body),
		})
		return
	}

	var raw struct {
		Items []struct {
			ID struct {
				VideoID string `json:"videoId"`
			} `json:"id"`
			Snippet struct {
				Title        string `json:"title"`
				ChannelTitle string `json:"channelTitle"`
				PublishedAt  string `json:"publishedAt"`
				Description  string `json:"description"`
				Thumbnails   struct {
					High struct {
						URL string `json:"url"`
					} `json:"high"`
					Medium struct {
						URL string `json:"url"`
					} `json:"medium"`
				} `json:"thumbnails"`
			} `json:"snippet"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "invalid YouTube response"})
		return
	}

	results := make([]map[string]any, 0, len(raw.Items))
	for _, item := range raw.Items {
		id := item.ID.VideoID
		if id == "" {
			continue
		}
		thumb := item.Snippet.Thumbnails.High.URL
		if thumb == "" {
			thumb = item.Snippet.Thumbnails.Medium.URL
		}
		results = append(results, map[string]any{
			"videoId":     id,
			"title":       item.Snippet.Title,
			"channel":     item.Snippet.ChannelTitle,
			"publishedAt": item.Snippet.PublishedAt,
			"description": item.Snippet.Description,
			"thumbnail":   thumb,
			"url":         "https://www.youtube.com/watch?v=" + id,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"configured": true,
		"query":      q,
		"results":    results,
	})
}

// youtubeVideo resolves a pasted YouTube link using the documented oEmbed
// endpoint. It does not require a Data API key and does not download media.
func youtubeVideo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rawURL := strings.TrimSpace(r.URL.Query().Get("url"))
	videoID := extractYouTubeVideoID(rawURL)
	if videoID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid YouTube URL"})
		return
	}

	canonicalURL := "https://www.youtube.com/watch?v=" + videoID
	oembedURL := "https://www.youtube.com/oembed?url=" + url.QueryEscape(canonicalURL) + "&format=json"
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, oembedURL, nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create YouTube metadata request"})
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not reach YouTube"})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error":  "YouTube video lookup failed",
			"detail": clean(body),
		})
		return
	}

	var meta struct {
		Title      string `json:"title"`
		AuthorName string `json:"author_name"`
		Thumbnail  string `json:"thumbnail_url"`
	}
	if err := json.Unmarshal(body, &meta); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "invalid YouTube metadata response"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"videoId":    videoID,
		"title":      meta.Title,
		"channel":    meta.AuthorName,
		"thumbnail":  meta.Thumbnail,
		"url":        canonicalURL,
		"searchMode": "keyless-oembed",
	})
}

func extractYouTubeVideoID(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if host == "youtu.be" {
		id := strings.Trim(u.Path, "/")
		if len(id) == 11 {
			return id
		}
		return ""
	}
	if host != "youtube.com" && host != "www.youtube.com" && host != "m.youtube.com" &&
		host != "youtube-nocookie.com" && host != "www.youtube-nocookie.com" {
		return ""
	}
	if id := u.Query().Get("v"); len(id) == 11 {
		return id
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i, part := range parts {
		if (part == "shorts" || part == "embed" || part == "live") && i+1 < len(parts) {
			id := parts[i+1]
			if len(id) == 11 {
				return id
			}
		}
	}
	return ""
}

func wineEnv() []string {
	remove := map[string]bool{
		"HOME": true, "WINEPREFIX": true, "WINEARCH": true,
		"WINEDEBUG": true, "TMPDIR": true,
	}

	env := make([]string, 0, len(os.Environ())+5)
	for _, item := range os.Environ() {
		key := item
		if i := strings.IndexByte(item, '='); i >= 0 {
			key = item[:i]
		}
		if remove[key] {
			continue
		}
		env = append(env, item)
	}

	return append(env,
		"HOME="+wineHome,
		"WINEPREFIX="+winePrefix,
		"WINEARCH=win64",
		"WINEDEBUG=-all",
		"TMPDIR=/tmp",
	)
}

func runWine(args ...string) ([]byte, error) {
	cmd := exec.Command("wine", args...)
	cmd.Env = wineEnv()
	return cmd.CombinedOutput()
}

func winePath(p string) string {
	p = filepath.ToSlash(filepath.Clean(p))
	return "Z:" + strings.ReplaceAll(p, "/", `\`)
}

func saveUpload(src multipart.File, dst string) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, src)
	return err
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func sanitizeFilename(s string) string {
	s = strings.ReplaceAll(s, `\`, "_")
	s = strings.ReplaceAll(s, `"`, "_")
	s = strings.ReplaceAll(s, "\r", "_")
	s = strings.ReplaceAll(s, "\n", "_")
	return s
}

func clean(b []byte) string {
	return strings.TrimSpace(string(b))
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
