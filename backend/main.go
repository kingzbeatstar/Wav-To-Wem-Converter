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
	log.Printf("wav-to-wem API V14 Extended starting")
	log.Printf("converter: Windows wav2wem.exe v0.1 via Wine")
	log.Printf("converter path: %s", converter)

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
	srv := &http.Server{Addr: ":" + port, Handler: cors(mux), ReadHeaderTimeout: 15 * time.Second}
	log.Printf("wav-to-wem API listening on :%s", port)
	log.Fatal(srv.ListenAndServe())
}

func root(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"service": "wav-to-wem-converter", "ok": true, "version": "v14-extended"})
}
func health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": "v14-extended"})
}
func diagnostics(w http.ResponseWriter, r *http.Request) {
	result := map[string]any{
		"version": "v14-extended", "wine_home": wineHome, "wine_prefix": winePrefix,
		"converter": converter, "converter_present": fileExists(converter),
		"prefix_present":         fileExists(winePrefix),
		"youtube_api_configured": youtubeAPIKey() != "",
	}
	out, err := runWine("--version")
	result["wine_available"] = err == nil
	result["wine_output"] = clean(out)
	if err != nil {
		result["wine_error"] = err.Error()
		writeJSON(w, 503, result)
		return
	}
	out, err = runWine(converter, "--help")
	result["wav2wem_self_test"] = err == nil
	result["wav2wem_output"] = clean(out)
	if err != nil {
		result["wav2wem_error"] = err.Error()
		writeJSON(w, 503, result)
		return
	}
	writeJSON(w, 200, result)
}

func convert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload+(1<<20))
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "missing multipart field 'file'"})
		return
	}
	defer file.Close()
	if header.Size > maxUpload {
		writeJSON(w, 413, map[string]string{"error": "file is larger than 200 MB"})
		return
	}
	if !strings.EqualFold(filepath.Ext(header.Filename), ".wav") {
		writeJSON(w, 400, map[string]string{"error": "only .wav files are accepted"})
		return
	}

	dir, err := os.MkdirTemp("", "wav2wem-api-")
	if err != nil {
		http.Error(w, "failed to create temp directory", 500)
		return
	}
	defer os.RemoveAll(dir)
	input, output := filepath.Join(dir, "input.wav"), filepath.Join(dir, "output.wem")
	if err := saveUpload(file, input); err != nil {
		http.Error(w, "failed to save upload", 500)
		return
	}

	args := []string{converter, winePath(input), "-o", winePath(output), "-q", "4"}
	start := time.Now()
	log.Printf("starting conversion: %s", header.Filename)
	cmd := exec.Command("wine", args...)
	cmd.Env = wineEnv()
	out, err := cmd.CombinedOutput()
	elapsed := time.Since(start)
	if cleanOut := clean(out); cleanOut != "" {
		log.Printf("wav2wem output: %s", cleanOut)
	}
	if err != nil {
		log.Printf("wav2wem failed after %s: %v", elapsed.Round(time.Millisecond), err)
		writeJSON(w, 422, map[string]string{"error": "WAV to WEM conversion failed", "detail": clean(out)})
		return
	}
	info, err := os.Stat(output)
	if err != nil || info.Size() == 0 {
		writeJSON(w, 500, map[string]string{"error": "WEM output missing", "detail": fmt.Sprintf("%v", err)})
		return
	}

	data, err := os.Open(output)
	if err != nil {
		http.Error(w, "failed to open WEM output", 500)
		return
	}
	defer data.Close()
	base := strings.TrimSuffix(filepath.Base(header.Filename), filepath.Ext(header.Filename))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, sanitizeFilename(base+".wem")))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
	w.WriteHeader(200)
	_, _ = io.Copy(w, data)
}

func youtubeAPIKey() string {
	if k := strings.TrimSpace(os.Getenv("YOUTUBE_API_KEY")); k != "" {
		return k
	}
	return strings.TrimSpace(os.Getenv("YT_API_KEY"))
}

func youtubeSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	key := youtubeAPIKey()
	if key == "" {
		writeJSON(w, 503, map[string]string{
			"error":  "Integrated YouTube search is not configured",
			"detail": "Add YOUTUBE_API_KEY to the Render environment variables, save it, and redeploy.",
		})
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, 400, map[string]string{"error": "missing search query"})
		return
	}
	if len(q) > 100 {
		writeJSON(w, 400, map[string]string{"error": "search query is too long"})
		return
	}

	p := url.Values{}
	p.Set("part", "snippet")
	p.Set("type", "video")
	p.Set("videoEmbeddable", "true")
	p.Set("maxResults", "10")
	p.Set("q", q)
	p.Set("key", key)
	req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://www.googleapis.com/youtube/v3/search?"+p.Encode(), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": "could not reach YouTube"})
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		writeJSON(w, 502, map[string]string{"error": "YouTube search failed", "detail": clean(body)})
		return
	}

	var sr struct {
		Items []struct {
			ID struct {
				VideoID string `json:"videoId"`
			} `json:"id"`
			Snippet struct {
				Title        string `json:"title"`
				Description  string `json:"description"`
				ChannelTitle string `json:"channelTitle"`
				PublishedAt  string `json:"publishedAt"`
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
	if json.Unmarshal(body, &sr) != nil {
		writeJSON(w, 502, map[string]string{"error": "invalid YouTube response"})
		return
	}

	ids := make([]string, 0, len(sr.Items))
	for _, item := range sr.Items {
		if item.ID.VideoID != "" {
			ids = append(ids, item.ID.VideoID)
		}
	}
	type vinfo struct {
		Duration   string
		Embeddable bool
	}
	details := map[string]vinfo{}
	if len(ids) > 0 {
		vp := url.Values{}
		vp.Set("part", "contentDetails,status")
		vp.Set("id", strings.Join(ids, ","))
		vp.Set("key", key)
		vreq, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://www.googleapis.com/youtube/v3/videos?"+vp.Encode(), nil)
		vresp, verr := http.DefaultClient.Do(vreq)
		if verr == nil {
			defer vresp.Body.Close()
			vbody, _ := io.ReadAll(io.LimitReader(vresp.Body, 1<<20))
			if vresp.StatusCode >= 200 && vresp.StatusCode < 300 {
				var vr struct {
					Items []struct {
						ID      string `json:"id"`
						Content struct {
							Duration string `json:"duration"`
						} `json:"contentDetails"`
						Status struct {
							Embeddable bool `json:"embeddable"`
						} `json:"status"`
					} `json:"items"`
				}
				if json.Unmarshal(vbody, &vr) == nil {
					for _, v := range vr.Items {
						details[v.ID] = vinfo{v.Content.Duration, v.Status.Embeddable}
					}
				}
			}
		}
	}
	results := make([]map[string]any, 0, len(sr.Items))
	for _, item := range sr.Items {
		id := item.ID.VideoID
		if id == "" {
			continue
		}
		inf, ok := details[id]
		if ok && !inf.Embeddable {
			continue
		}
		thumb := item.Snippet.Thumbnails.High.URL
		if thumb == "" {
			thumb = item.Snippet.Thumbnails.Medium.URL
		}
		results = append(results, map[string]any{"videoId": id, "title": item.Snippet.Title, "channel": item.Snippet.ChannelTitle, "description": item.Snippet.Description, "publishedAt": item.Snippet.PublishedAt, "thumbnail": thumb, "duration": inf.Duration, "embeddable": true, "url": "https://www.youtube.com/watch?v=" + id})
	}
	writeJSON(w, 200, map[string]any{"configured": true, "query": q, "results": results})
}

func youtubeVideo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	raw := strings.TrimSpace(r.URL.Query().Get("url"))
	id := extractYouTubeVideoID(raw)
	if id == "" {
		writeJSON(w, 400, map[string]string{"error": "invalid YouTube URL"})
		return
	}
	canonical := "https://www.youtube.com/watch?v=" + id

	oe := "https://www.youtube.com/oembed?url=" + url.QueryEscape(canonical) + "&format=json"
	req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, oe, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": "could not reach YouTube"})
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		writeJSON(w, 502, map[string]string{"error": "YouTube video lookup failed", "detail": clean(body)})
		return
	}
	var meta struct {
		Title        string `json:"title"`
		AuthorName   string `json:"author_name"`
		ThumbnailURL string `json:"thumbnail_url"`
	}
	if json.Unmarshal(body, &meta) != nil {
		writeJSON(w, 502, map[string]string{"error": "invalid YouTube metadata response"})
		return
	}

	result := map[string]any{"videoId": id, "title": meta.Title, "channel": meta.AuthorName, "thumbnail": meta.ThumbnailURL, "url": canonical, "embeddable": true}
	if key := youtubeAPIKey(); key != "" {
		vp := url.Values{}
		vp.Set("part", "contentDetails,status")
		vp.Set("id", id)
		vp.Set("key", key)
		vreq, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://www.googleapis.com/youtube/v3/videos?"+vp.Encode(), nil)
		vresp, verr := http.DefaultClient.Do(vreq)
		if verr == nil {
			defer vresp.Body.Close()
			vb, _ := io.ReadAll(io.LimitReader(vresp.Body, 1<<20))
			if vresp.StatusCode >= 200 && vresp.StatusCode < 300 {
				var vr struct {
					Items []struct {
						Content struct {
							Duration string `json:"duration"`
						} `json:"contentDetails"`
						Status struct {
							Embeddable bool `json:"embeddable"`
						} `json:"status"`
					} `json:"items"`
				}
				if json.Unmarshal(vb, &vr) == nil && len(vr.Items) > 0 {
					result["duration"] = vr.Items[0].Content.Duration
					result["embeddable"] = vr.Items[0].Status.Embeddable
					if !vr.Items[0].Status.Embeddable {
						result["embedMessage"] = "This video owner does not allow embedding. Choose another video."
					}
				}
			}
		}
	}
	writeJSON(w, 200, result)
}

func extractYouTubeVideoID(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	h := strings.ToLower(u.Hostname())
	if h == "youtu.be" {
		id := strings.Trim(u.Path, "/")
		if len(id) == 11 {
			return id
		}
		return ""
	}
	if h != "youtube.com" && h != "www.youtube.com" && h != "m.youtube.com" && h != "youtube-nocookie.com" && h != "www.youtube-nocookie.com" {
		return ""
	}
	if id := u.Query().Get("v"); len(id) == 11 {
		return id
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i, p := range parts {
		if (p == "shorts" || p == "embed" || p == "live") && i+1 < len(parts) {
			id := parts[i+1]
			if len(id) == 11 {
				return id
			}
		}
	}
	return ""
}

func wineEnv() []string {
	remove := map[string]bool{"HOME": true, "WINEPREFIX": true, "WINEARCH": true, "WINEDEBUG": true, "TMPDIR": true}
	env := make([]string, 0, len(os.Environ())+5)
	for _, item := range os.Environ() {
		key := item
		if i := strings.IndexByte(item, '='); i >= 0 {
			key = item[:i]
		}
		if !remove[key] {
			env = append(env, item)
		}
	}
	return append(env, "HOME="+wineHome, "WINEPREFIX="+winePrefix, "WINEARCH=win64", "WINEDEBUG=-all", "TMPDIR=/tmp")
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
func fileExists(p string) bool { info, err := os.Stat(p); return err == nil && !info.IsDir() }
func sanitizeFilename(s string) string {
	s = strings.ReplaceAll(s, `\`, "_")
	s = strings.ReplaceAll(s, `"`, "_")
	s = strings.ReplaceAll(s, "\r", "_")
	s = strings.ReplaceAll(s, "\n", "_")
	return s
}
func clean(b []byte) string { return strings.TrimSpace(string(b)) }
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
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
