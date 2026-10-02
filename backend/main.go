package main

import (
	"bufio"
	"context"
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
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maxUpload = 200 << 20 // 200 MiB
const converter = "/opt/wav2wem/wav2wem.exe"
const ytdlp = "/opt/yt-dlp/yt-dlp"
const youtubeCookiesPath = "/etc/secrets/youtube-cookies.txt"
const deno = "/opt/yt-dlp/deno"
const bgutilPing = "http://127.0.0.1:4416/ping"
const wineHome = "/home/appuser"
const winePrefix = "/home/appuser/.wine"
const maxYouTubeWAV = 200 << 20 // Keep generated WAV within the same 200 MB service limit.
const jobTTL = 45 * time.Minute

var (
	jobsMu sync.RWMutex
	jobs   = map[string]*YouTubeJob{}
	// Free Render instances are intentionally kept to one active YouTube job at a time.
	youtubeSlots = make(chan struct{}, 1)
	percentRE    = regexp.MustCompile(`PROGRESS=\s*([0-9]+(?:\.[0-9]+)?)%`)
)

type YouTubeJob struct {
	ID           string
	VideoID      string
	Title        string
	Status       string // queued, downloading, converting, complete, error
	Stage        string
	Progress     int
	Error        string
	WEMPath      string
	DownloadName string
	CreatedAt    time.Time
	FinishedAt   time.Time
}

type youtubeJobRequest struct {
	VideoID string `json:"videoId"`
	URL     string `json:"url"`
	Confirm bool   `json:"confirm"`
}

func main() {
	log.Printf("wav-to-wem API V21 Audio-Fix starting")
	log.Printf("converter: Windows wav2wem.exe v0.1 via Wine")
	log.Printf("converter path: %s", converter)
	log.Printf("yt-dlp: %s", ytdlp)
	log.Printf("Deno: %s", deno)

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

	if !fileExists(ytdlp) {
		log.Fatalf("yt-dlp missing: %s", ytdlp)
	}
	if !fileExists(deno) {
		log.Fatalf("Deno missing: %s", deno)
	}
	if out, err := execCommand(ytdlp, "--version"); err != nil {
		log.Fatalf("YTDLP SELF-TEST FAILED: %v: %s", err, clean(out))
	} else {
		log.Printf("YTDLP SELF-TEST OK: %s", clean(out))
	}
	if out, err := execCommand(deno, "--version"); err != nil {
		log.Fatalf("DENO SELF-TEST FAILED: %v: %s", err, clean(out))
	} else {
		log.Printf("DENO SELF-TEST OK: %s", firstLine(clean(out)))
	}
	if out, err := execCommand("curl", "-fsS", bgutilPing); err != nil {
		log.Fatalf("BGUTIL POT PROVIDER SELF-TEST FAILED: %v: %s", err, clean(out))
	} else {
		log.Printf("BGUTIL POT PROVIDER SELF-TEST OK: %s", clean(out))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", root)
	mux.HandleFunc("/health", health)
	mux.HandleFunc("/diagnostics", diagnostics)
	mux.HandleFunc("/convert", convert)
	mux.HandleFunc("/youtube/search", youtubeSearch)
	mux.HandleFunc("/youtube/video", youtubeVideo)
	mux.HandleFunc("/youtube/convert", youtubeConvert)
	mux.HandleFunc("/youtube/job", youtubeJobStatus)
	mux.HandleFunc("/youtube/download", youtubeDownload)

	port := os.Getenv("PORT")
	if port == "" {
		port = "10000"
	}
	srv := &http.Server{Addr: ":" + port, Handler: cors(mux), ReadHeaderTimeout: 15 * time.Second}
	log.Printf("wav-to-wem API listening on :%s", port)
	log.Fatal(srv.ListenAndServe())
}

func root(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service": "wav-to-wem-converter",
		"ok":      true,
		"version": "v21-audiofix",
	})
}

func health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": "v21-audiofix"})
}

func diagnostics(w http.ResponseWriter, r *http.Request) {
	result := map[string]any{
		"version":                "v21-audiofix",
		"wine_home":              wineHome,
		"wine_prefix":            winePrefix,
		"converter":              converter,
		"converter_present":      fileExists(converter),
		"prefix_present":         fileExists(winePrefix),
		"yt_dlp_present":         fileExists(ytdlp),
		"deno_present":           fileExists(deno),
		"youtube_api_configured": youtubeAPIKey() != "",
		"youtube_cookies_present": fileExists(youtubeCookiesPath),
		"bgutil_provider": bgutilPing,
		"ffmpeg_present": fileExists("/usr/bin/ffmpeg"),
		"ffprobe_present": fileExists("/usr/bin/ffprobe"),
	}
	out, err := runWine("--version")
	result["wine_available"] = err == nil
	result["wine_output"] = clean(out)
	if err != nil {
		result["wine_error"] = err.Error()
		writeJSON(w, http.StatusServiceUnavailable, result)
		return
	}
	out, err = runWine(converter, "--help")
	result["wav2wem_self_test"] = err == nil
	result["wav2wem_output"] = clean(out)
	if err != nil {
		result["wav2wem_error"] = err.Error()
		writeJSON(w, http.StatusServiceUnavailable, result)
		return
	}
	out, err = execCommand(ytdlp, "--version")
	result["ytdlp_self_test"] = err == nil
	result["ytdlp_output"] = clean(out)
	out, err = execCommand(deno, "--version")
	result["deno_self_test"] = err == nil
	result["deno_output"] = clean(out)
	out, err = execCommand("curl", "-fsS", bgutilPing)
	result["bgutil_self_test"] = err == nil
	result["bgutil_output"] = clean(out)
	if err != nil {
		result["bgutil_error"] = err.Error()
	}
	writeJSON(w, http.StatusOK, result)
}

func convert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload+(1<<20))
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing multipart field 'file'"})
		return
	}
	defer file.Close()
	if header.Size > maxUpload {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "file is larger than 200 MB"})
		return
	}
	if !strings.EqualFold(filepath.Ext(header.Filename), ".wav") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "only .wav files are accepted"})
		return
	}

	dir, err := os.MkdirTemp("", "wav2wem-api-")
	if err != nil {
		http.Error(w, "failed to create temp directory", http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(dir)
	input, output := filepath.Join(dir, "input.wav"), filepath.Join(dir, "output.wem")
	if err := saveUpload(file, input); err != nil {
		http.Error(w, "failed to save upload", http.StatusInternalServerError)
		return
	}

	wem, err := runWavToWem(input, output)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "WAV to WEM conversion failed", "detail": err.Error()})
		return
	}
	data, err := os.Open(wem)
	if err != nil {
		http.Error(w, "failed to open WEM output", http.StatusInternalServerError)
		return
	}
	defer data.Close()
	info, _ := data.Stat()
	base := strings.TrimSuffix(filepath.Base(header.Filename), filepath.Ext(header.Filename))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, sanitizeFilename(base+".wem")))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, data)
}

func youtubeConvert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req youtubeJobRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if !req.Confirm {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "confirm that you are authorized to download/use this audio"})
		return
	}
	videoID := strings.TrimSpace(req.VideoID)
	if videoID == "" {
		videoID = extractYouTubeVideoID(req.URL)
	}
	if !validYouTubeID(videoID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid YouTube video"})
		return
	}

	title, thumbnail, err := youtubeOEmbed(r.Context(), videoID)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not identify YouTube video", "detail": err.Error()})
		return
	}
	jobID := randomID()
	job := &YouTubeJob{
		ID: jobID, VideoID: videoID, Title: title, Status: "queued",
		Stage: "Queued — waiting for the cloud converter…", Progress: 0,
		CreatedAt: time.Now(),
	}
	jobsMu.Lock()
	jobs[jobID] = job
	jobsMu.Unlock()

	go runYouTubeJob(job, thumbnail)

	writeJSON(w, http.StatusAccepted, map[string]any{
		"jobId":     jobID,
		"status":    "queued",
		"title":     title,
		"thumbnail": thumbnail,
		"poll":      "/youtube/job?id=" + url.QueryEscape(jobID),
		"download":  "/youtube/download?id=" + url.QueryEscape(jobID),
	})
}

func runYouTubeJob(job *YouTubeJob, thumbnail string) {
	youtubeSlots <- struct{}{}
	defer func() { <-youtubeSlots }()

	updateJob(job.ID, func(j *YouTubeJob) {
		j.Status = "downloading"
		j.Stage = "Downloading audio from YouTube and converting it to WAV…"
		j.Progress = 5
	})

	dir, err := os.MkdirTemp("", "yt2wem-")
	if err != nil {
		failJob(job.ID, "Could not create a temporary conversion directory.")
		return
	}
	// Keep the directory until the WEM is downloaded.

	wavTemplate := filepath.Join(dir, "source.%(ext)s")
	videoURL := "https://www.youtube.com/watch?v=" + job.VideoID

	// V20 uses an audio-specific selector and multiple current YouTube client
	// routes. This avoids yt-dlp's normal bestvideo+bestaudio selector and also
	// avoids forcing web_embedded, which can expose no downloadable A/V formats
	// for some videos.
	type ytAttempt struct {
		Name    string
		Clients string
	}
	attempts := []ytAttempt{
		{Name: "recommended default+mweb route", Clients: "default,mweb"},
		{Name: "mweb PO-token route", Clients: "mweb"},
		{Name: "Safari/HLS compatibility route", Clients: "web_safari"},
		{Name: "yt-dlp automatic client route", Clients: ""},
	}

	buildArgs := func(clients string) []string {
		args := []string{
			"--no-playlist",
			"--no-warnings",
			"--newline",
			"--progress",
			"--progress-delta", "1",
			"--progress-template", "download:PROGRESS=%(progress._percent_str)s",
			"--js-runtimes", "deno:/opt/yt-dlp/deno",
			"--extractor-args", "youtubepot-bgutilhttp:base_url=http://127.0.0.1:4416",
			// Explicitly ask for the best audio stream first. If YouTube exposes
			// only a combined stream, fall back to the best stream containing audio.
			"--format", "bestaudio/best",
			"--sleep-requests", "1",
			"--sleep-interval", "5",
			"--max-sleep-interval", "10",
			"--extract-audio",
			"--audio-format", "wav",
			"--audio-quality", "0",
			"--max-filesize", "200M",
			"--restrict-filenames",
			"--output", wavTemplate,
		}
		if clients != "" {
			args = append(args, "--extractor-args", "youtube:player-client="+clients)
		}
		if fileExists(youtubeCookiesPath) {
			args = append(args, "--cookies", youtubeCookiesPath)
			if ua := strings.TrimSpace(os.Getenv("YOUTUBE_USER_AGENT")); ua != "" {
				args = append(args, "--user-agent", ua)
			}
		}
		return append(args, videoURL)
	}

	var (
		downloadOK  bool
		lastOutput  []byte
		lastErr     error
	)
	for i, attempt := range attempts {
		// Remove partial media from a previous route before retrying.
		if matches, _ := filepath.Glob(filepath.Join(dir, "source.*")); len(matches) > 0 {
			for _, p := range matches {
				_ = os.Remove(p)
			}
		}

		updateJob(job.ID, func(j *YouTubeJob) {
			if i == 0 {
				j.Stage = "Downloading the best available YouTube audio…"
			} else {
				j.Stage = fmt.Sprintf("YouTube compatibility fallback %d/%d — %s…", i+1, len(attempts), attempt.Name)
			}
			if j.Progress < 5+i*2 {
				j.Progress = 5 + i*2
			}
		})

		if fileExists(youtubeCookiesPath) {
			log.Printf("YouTube job %s: attempt %d/%d (%s), authenticated cookie jar enabled", job.ID, i+1, len(attempts), attempt.Name)
		} else {
			log.Printf("YouTube job %s: attempt %d/%d (%s), anonymous session", job.ID, i+1, len(attempts), attempt.Name)
		}

		out, runErr := runCommandStreaming(ytdlp, buildArgs(attempt.Clients), func(line string) {
			if m := percentRE.FindStringSubmatch(line); len(m) == 2 {
				if p, e := strconv.ParseFloat(m[1], 64); e == nil {
					mapped := 5 + int(p*0.65)
					if mapped > 70 {
						mapped = 70
					}
					updateJob(job.ID, func(j *YouTubeJob) {
						if mapped > j.Progress {
							j.Progress = mapped
						}
					})
				}
			}
		})
		lastOutput, lastErr = out, runErr

		if runErr == nil {
			downloadOK = true
			log.Printf("YouTube job %s: attempt %d succeeded (%s)", job.ID, i+1, attempt.Name)
			break
		}

		detail := clean(out)
		log.Printf("YouTube job %s: attempt %d failed (%s): %v: %s", job.ID, i+1, attempt.Name, runErr, detail)

		// Sometimes post-processing reports a non-zero exit after already
		// producing a valid WAV. Keep it instead of needlessly failing.
		if wav, wavErr := findWAV(dir); wavErr == nil {
			if info, statErr := os.Stat(wav); statErr == nil && info.Size() > 44 {
				if _, probeErr := execCommand("ffprobe", "-v", "error", "-show_entries", "format=duration",
					"-of", "default=noprint_wrappers=1:nokey=1", wav); probeErr == nil {
					downloadOK = true
					log.Printf("YouTube job %s: keeping valid WAV produced despite yt-dlp non-zero exit", job.ID)
					break
				}
			}
		}

		if !youtubeRouteRetryable(detail) || i == len(attempts)-1 {
			break
		}

		// Small cooldown before changing clients. This reduces repeated rapid
		// requests against the same video/session on Render.
		time.Sleep(2 * time.Second)
	}

	if !downloadOK {
		log.Printf("YouTube job %s: all applicable yt-dlp routes failed: %v: %s", job.ID, lastErr, clean(lastOutput))
		failJob(job.ID, youtubeFriendlyError(clean(lastOutput)))
		os.RemoveAll(dir)
		return
	}

	wavPath, err := findWAV(dir)
	if err != nil {
		failJob(job.ID, err.Error())
		os.RemoveAll(dir)
		return
	}
	if info, e := os.Stat(wavPath); e != nil {
		failJob(job.ID, "Downloaded WAV could not be inspected.")
		os.RemoveAll(dir)
		return
	} else if info.Size() > maxYouTubeWAV {
		failJob(job.ID, "The generated WAV is larger than 200 MB. Choose a shorter/lower-size source.")
		os.RemoveAll(dir)
		return
	}

	updateJob(job.ID, func(j *YouTubeJob) {
		j.Status = "converting"
		j.Stage = "Rendering Beatstar-safe 44.1 kHz stereo PCM, then converting to WEM…"
		j.Progress = 72
	})

	wemPath := filepath.Join(dir, "output.wem")
	if _, err := runWavToWem(wavPath, wemPath); err != nil {
		failJob(job.ID, err.Error())
		os.RemoveAll(dir)
		return
	}

	downloadName := sanitizeFilename(safeTitle(job.Title) + ".wem")
	updateJob(job.ID, func(j *YouTubeJob) {
		j.Status = "complete"
		j.Stage = "Complete — your WEM is ready."
		j.Progress = 100
		j.WEMPath = wemPath
		j.DownloadName = downloadName
		j.FinishedAt = time.Now()
	})

	// Remove abandoned completed jobs after a while.
	time.AfterFunc(jobTTL, func() { deleteJob(job.ID) })
}

func youtubeJobStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	job := getJob(id)
	if job == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found or expired"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"jobId": job.ID, "videoId": job.VideoID, "title": job.Title,
		"status": job.Status, "stage": job.Stage, "progress": job.Progress,
		"error":    job.Error,
		"ready":    job.Status == "complete",
		"download": "/youtube/download?id=" + url.QueryEscape(job.ID),
	})
}

func youtubeDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	job := getJob(id)
	if job == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found or expired"})
		return
	}
	if job.Status != "complete" || job.WEMPath == "" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "WEM is not ready yet"})
		return
	}
	file, err := os.Open(job.WEMPath)
	if err != nil {
		writeJSON(w, http.StatusGone, map[string]string{"error": "WEM file expired; convert again"})
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		http.Error(w, "failed to stat WEM", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, job.DownloadName))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, file)
	if job.Status == "complete" {
		// A completed job is one-shot by design; remove its temp directory after serving.
		time.AfterFunc(15*time.Second, func() { deleteJob(id) })
	}
}

func youtubeSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	key := youtubeAPIKey()
	if key == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error":  "Integrated YouTube search is not configured",
			"detail": "Add YOUTUBE_API_KEY to the Render environment variables and redeploy.",
		})
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing search query"})
		return
	}
	p := url.Values{}
	p.Set("part", "snippet")
	p.Set("type", "video")
	p.Set("videoEmbeddable", "true")
	p.Set("maxResults", "10")
	p.Set("q", q)
	p.Set("key", key)
	resp, err := http.DefaultClient.Get("https://www.googleapis.com/youtube/v3/search?" + p.Encode())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not reach YouTube"})
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "YouTube search failed", "detail": clean(body)})
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
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "invalid YouTube response"})
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
		vresp, verr := http.DefaultClient.Get("https://www.googleapis.com/youtube/v3/videos?" + vp.Encode())
		if verr == nil {
			defer vresp.Body.Close()
			vb, _ := io.ReadAll(io.LimitReader(vresp.Body, 1<<20))
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
				if json.Unmarshal(vb, &vr) == nil {
					for _, v := range vr.Items {
						details[v.ID] = vinfo{Duration: v.Content.Duration, Embeddable: v.Status.Embeddable}
					}
				}
			}
		}
	}

	results := make([]map[string]any, 0, len(sr.Items))
	for _, item := range sr.Items {
		id := item.ID.VideoID
		vi, ok := details[id]
		if id == "" || (ok && !vi.Embeddable) {
			continue
		}
		thumb := item.Snippet.Thumbnails.High.URL
		if thumb == "" {
			thumb = item.Snippet.Thumbnails.Medium.URL
		}
		results = append(results, map[string]any{
			"videoId": id, "title": item.Snippet.Title, "channel": item.Snippet.ChannelTitle,
			"description": item.Snippet.Description, "publishedAt": item.Snippet.PublishedAt,
			"thumbnail": thumb, "duration": vi.Duration,
			"url": "https://www.youtube.com/watch?v=" + id, "embeddable": true,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"configured": true, "query": q, "results": results})
}

func youtubeVideo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	videoID := extractYouTubeVideoID(strings.TrimSpace(r.URL.Query().Get("url")))
	if !validYouTubeID(videoID) {
		writeJSON(w, 400, map[string]string{"error": "invalid YouTube URL"})
		return
	}
	title, thumb, err := youtubeOEmbed(r.Context(), videoID)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": "could not reach YouTube", "detail": err.Error()})
		return
	}
	result := map[string]any{"videoId": videoID, "title": title, "thumbnail": thumb, "url": "https://www.youtube.com/watch?v=" + videoID, "embeddable": true}
	if key := youtubeAPIKey(); key != "" {
		p := url.Values{}
		p.Set("part", "contentDetails,status")
		p.Set("id", videoID)
		p.Set("key", key)
		resp, err := http.DefaultClient.Get("https://www.googleapis.com/youtube/v3/videos?" + p.Encode())
		if err == nil {
			defer resp.Body.Close()
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
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
				if json.Unmarshal(body, &vr) == nil && len(vr.Items) > 0 {
					result["duration"] = vr.Items[0].Content.Duration
					result["embeddable"] = vr.Items[0].Status.Embeddable
					if !vr.Items[0].Status.Embeddable {
						result["embedMessage"] = "This video does not allow embedding. Choose another video."
					}
				}
			}
		}
	}
	writeJSON(w, 200, result)
}

func youtubeOEmbed(ctx context.Context, videoID string) (string, string, error) {
	canonical := "https://www.youtube.com/watch?v=" + videoID
	u := "https://www.youtube.com/oembed?url=" + url.QueryEscape(canonical) + "&format=json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("YouTube metadata returned HTTP %d", resp.StatusCode)
	}
	var meta struct {
		Title     string `json:"title"`
		Thumbnail string `json:"thumbnail_url"`
	}
	if err := json.Unmarshal(body, &meta); err != nil {
		return "", "", err
	}
	return meta.Title, meta.Thumbnail, nil
}

func runWavToWem(input, output string) (string, error) {
	// Beatstar's known-good music WEMs use a conventional stereo/44.1 kHz
	// source profile. YouTube commonly supplies 48 kHz Opus/AAC, and WAV can
	// also be float/24-bit/multichannel. wav2wem may accept those inputs while
	// the resulting media is not usable by the game's playback path.
	//
	// Always render a clean intermediate PCM WAV first:
	//   PCM signed 16-bit little-endian, stereo, 44.1 kHz.
	normalized := filepath.Join(filepath.Dir(output), "beatstar_pcm_44100_stereo_s16.wav")
	if err := normalizeBeatstarWAV(input, normalized); err != nil {
		return "", err
	}
	defer os.Remove(normalized)

	args := []string{converter, winePath(normalized), "-o", winePath(output), "-q", "4"}
	out, err := runCommandStreaming("wine", args, func(string) {})
	if err != nil {
		return "", fmt.Errorf("wav2wem: %s", clean(out))
	}
	if info, e := os.Stat(output); e != nil || info.Size() <= 44 {
		return "", fmt.Errorf("converter completed but no valid WEM was produced")
	}
	return output, nil
}

func normalizeBeatstarWAV(input, output string) error {
	args := []string{
		"-nostdin",
		"-hide_banner",
		"-loglevel", "error",
		"-y",
		"-i", input,
		"-map", "0:a:0",
		"-vn",
		"-sn",
		"-dn",
		"-ac", "2",
		"-ar", "44100",
		"-c:a", "pcm_s16le",
		"-map_metadata", "-1",
		output,
	}
	out, err := execCommand("ffmpeg", args...)
	if err != nil {
		return fmt.Errorf("Beatstar PCM rendering failed: %s", clean(out))
	}

	info, err := os.Stat(output)
	if err != nil || info.Size() <= 44 {
		return fmt.Errorf("Beatstar PCM rendering produced no usable audio")
	}

	// Verify the exact audio contract before handing it to wav2wem.
	probe, err := execCommand(
		"ffprobe",
		"-v", "error",
		"-select_streams", "a:0",
		"-show_entries", "stream=codec_name,sample_rate,channels,bits_per_sample",
		"-of", "default=noprint_wrappers=1",
		output,
	)
	if err != nil {
		return fmt.Errorf("could not verify rendered WAV: %s", clean(probe))
	}
	p := strings.ToLower(clean(probe))
	if !strings.Contains(p, "codec_name=pcm_s16le") ||
		!strings.Contains(p, "sample_rate=44100") ||
		!strings.Contains(p, "channels=2") {
		return fmt.Errorf("rendered WAV did not match Beatstar-safe PCM profile: %s", clean(probe))
	}

	// Detect the pathological case where a pipeline technically produced a WAV
	// but its samples are entirely silent.
	volume, _ := execCommand(
		"ffmpeg",
		"-nostdin",
		"-hide_banner",
		"-i", output,
		"-af", "volumedetect",
		"-f", "null",
		"-",
	)
	if strings.Contains(strings.ToLower(clean(volume)), "max_volume: -inf") {
		return fmt.Errorf("rendered WAV contains only silence; conversion stopped before creating a silent WEM")
	}

	log.Printf("Beatstar PCM render OK: pcm_s16le, stereo, 44100 Hz (%d bytes)", info.Size())
	return nil
}

func findWAV(dir string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.wav"))
	if err != nil || len(matches) == 0 {
		return "", fmt.Errorf("YouTube audio download completed, but no WAV file was produced")
	}
	return matches[0], nil
}

func youtubeRouteRetryable(detail string) bool {
	lower := strings.ToLower(detail)

	// These failures can genuinely differ by YouTube client/transport, so V20
	// retries them through a small set of current compatibility routes.
	retryable := []string{
		"requested format is not available",
		"no formats found",
		"no video formats found",
		"only images are available",
		"no downloadable formats",
		"http error 403",
		"403: forbidden",
		"forbidden",
	}
	for _, needle := range retryable {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

func youtubeFriendlyError(detail string) string {
	lower := strings.ToLower(detail)
	if strings.Contains(lower, "po token") || strings.Contains(lower, "proof of origin") {
		return "YouTube rejected the stream token request. The PO-token provider is enabled, but this video/session was still refused."
	}
	if strings.Contains(lower, "sign in") || strings.Contains(lower, "confirm you’re not a bot") || strings.Contains(lower, "confirm you're not a bot") {
		if fileExists(youtubeCookiesPath) {
			return "YouTube still rejected the authenticated server session. Refresh the Render secret file youtube-cookies.txt from a fresh dedicated browser session, then redeploy. Do not paste cookies into the website, GitHub, logs, or chat."
		}
		return "YouTube is requiring an authenticated browser session for this Render IP. Add a Render Secret File named youtube-cookies.txt, then redeploy. The backend will use it only server-side."
	}
	if strings.Contains(lower, "http error 429") || strings.Contains(lower, "too many requests") {
		return "YouTube rate-limited this Render IP/session. V21 already slows requests; wait and try again later."
	}
	if strings.Contains(lower, "http error 403") {
		return "YouTube returned HTTP 403 for the media stream. This can still happen on datacenter IPs even with tokens/cookies; retry later or use another authorized source."
	}
	if strings.Contains(lower, "requested format is not available") || strings.Contains(lower, "no formats found") {
		return "YouTube did not expose a downloadable audio stream after V20 tried its recommended mweb/default, mweb-only, Safari/HLS, and automatic client routes. If this happens only on one video, that video is currently restricted for server-side extraction; if it happens on every video, refresh the YouTube cookie secret."
	}
	return "YouTube audio conversion failed: " + detail
}

func runCommandStreaming(name string, args []string, onLine func(string)) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = baseEnv()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var mu sync.Mutex
	var all []string
	read := func(r io.Reader) {
		s := bufio.NewScanner(r)
		for s.Scan() {
			line := s.Text()
			mu.Lock()
			all = append(all, line)
			mu.Unlock()
			onLine(line)
		}
	}
	done := make(chan struct{}, 2)
	go func() { read(stdout); done <- struct{}{} }()
	go func() { read(stderr); done <- struct{}{} }()
	<-done
	<-done
	waitErr := cmd.Wait()
	return []byte(strings.Join(all, "\n")), waitErr
}

func execCommand(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = baseEnv()
	return cmd.CombinedOutput()
}

func baseEnv() []string {
	remove := map[string]bool{"HOME": true, "WINEPREFIX": true, "WINEARCH": true, "WINEDEBUG": true, "TMPDIR": true, "XDG_CACHE_HOME": true}
	env := make([]string, 0, len(os.Environ())+6)
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
		"DENO_NO_UPDATE_CHECK=1",
	)
}

func runWine(args ...string) ([]byte, error) { return execCommand("wine", args...) }

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
	if s == ".wem" || s == "" {
		s = "youtube.wem"
	}
	return s
}
func safeTitle(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "youtube-audio"
	}
	if len(s) > 120 {
		s = s[:120]
	}
	return strings.TrimSpace(s)
}
func clean(b []byte) string { return strings.TrimSpace(string(b)) }
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}
func validYouTubeID(s string) bool {
	if len(s) != 11 {
		return false
	}
	for _, r := range s {
		if !(r == '-' || r == '_' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}
func extractYouTubeVideoID(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if host == "youtu.be" {
		id := strings.Trim(u.Path, "/")
		if validYouTubeID(id) {
			return id
		}
		return ""
	}
	if host != "youtube.com" && host != "www.youtube.com" && host != "m.youtube.com" && host != "youtube-nocookie.com" && host != "www.youtube-nocookie.com" {
		return ""
	}
	if id := u.Query().Get("v"); validYouTubeID(id) {
		return id
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i, p := range parts {
		if (p == "shorts" || p == "embed" || p == "live") && i+1 < len(parts) && validYouTubeID(parts[i+1]) {
			return parts[i+1]
		}
	}
	return ""
}
func youtubeAPIKey() string {
	if k := strings.TrimSpace(os.Getenv("YOUTUBE_API_KEY")); k != "" {
		return k
	}
	return strings.TrimSpace(os.Getenv("YT_API_KEY"))
}
func randomID() string { return fmt.Sprintf("%d-%d", time.Now().UnixNano(), os.Getpid()) }

func updateJob(id string, fn func(*YouTubeJob)) {
	jobsMu.Lock()
	defer jobsMu.Unlock()
	if j := jobs[id]; j != nil {
		fn(j)
	}
}
func failJob(id, msg string) {
	updateJob(id, func(j *YouTubeJob) {
		j.Status = "error"
		j.Error = msg
		j.Stage = "Conversion failed."
		j.FinishedAt = time.Now()
	})
	time.AfterFunc(10*time.Minute, func() { deleteJob(id) })
}
func getJob(id string) *YouTubeJob {
	jobsMu.RLock()
	defer jobsMu.RUnlock()
	j := jobs[id]
	if j == nil {
		return nil
	}
	cp := *j
	return &cp
}
func deleteJob(id string) {
	jobsMu.Lock()
	j := jobs[id]
	delete(jobs, id)
	jobsMu.Unlock()
	if j != nil && j.WEMPath != "" {
		os.RemoveAll(filepath.Dir(j.WEMPath))
	}
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
