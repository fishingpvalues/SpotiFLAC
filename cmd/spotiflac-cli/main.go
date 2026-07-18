// Headless CLI for SpotiFLAC — used by spotiflac-lidarr-proxy.
// Build: go build -tags headless ./cmd/spotiflac-cli
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"spotiflac/backend"
)

func main() {
	url := flag.String("url", "", "Spotify URL to download (track/album/playlist)")
	outputDir := flag.String("output-dir", ".", "Output directory for downloaded files")
	service := flag.String("service", "tidal", "Download service: tidal, qobuz, amazon, deezer")
	quality := flag.String("quality", "lossless", "Quality: lossless, hires, both")
	format := flag.String("format", "{artist}/{album}/{track} - {title}.flac", "Filename format")
	jsonProgress := flag.Bool("json-progress", false, "Emit JSON progress lines on stdout")
	search := flag.String("search", "", "Search Spotify for metadata (instead of downloading)")
	timeout := flag.Duration("timeout", 30*time.Minute, "Max download time")
	flag.Parse()

	if *search != "" {
		results, err := searchSpotify(*search)
		if err != nil {
			fmt.Fprintf(os.Stderr, "search error: %s\n", err)
			os.Exit(1)
		}
		for _, r := range results {
			json.NewEncoder(os.Stdout).Encode(r)
		}
		return
	}

	if *url == "" {
		fmt.Fprintln(os.Stderr, "error: --url is required")
		flag.Usage()
		os.Exit(1)
	}

	// Initialize backend
	_ = backend.InitHistoryDB("SpotiFLAC-CLI")
	_ = backend.InitISRCCacheDB()
	_ = backend.CleanupLegacyTidalPublicAPIState()
	_ = backend.SanitizePersistedConfigSettings()

	app := &App{}
	app.ctx = context.Background()

	// Resolve Spotify metadata
	metadataJSON, err := app.GetSpotifyMetadata(SpotifyMetadataRequest{
		URL:   *url,
		Batch: true,
	})
	if err != nil {
		emitError(*jsonProgress, fmt.Sprintf("metadata: %s", err))
		os.Exit(1)
	}

	var metadata SpotifyMetadata
	if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
		emitError(*jsonProgress, fmt.Sprintf("parse metadata: %s", err))
		os.Exit(1)
	}

	// Download each track
	for i, track := range metadata.Tracks {
		emitProgress(*jsonProgress, ProgressEvent{
			Type:    "progress",
			Track:   fmt.Sprintf("%02d", i+1),
			Title:   track.Name,
			Percent: float64(i) / float64(len(metadata.Tracks)) * 100,
		})

		req := DownloadRequest{
			Service:      *service,
			TrackName:    track.Name,
			ArtistName:   track.Artist,
			AlbumName:    metadata.Title,
			AlbumArtist:  metadata.Artist,
			ReleaseDate:  metadata.ReleaseDate,
			CoverURL:     metadata.CoverURL,
			OutputDir:    *outputDir,
			FilenameFormat: *format,
			SpotifyID:    track.ID,
			ISRC:         track.ISRC,
			Duration:     track.Duration,
			Position:     i + 1,
			SaveCover:    true,
			EmbedLyrics:  true,
		}

		resp, err := app.DownloadTrack(req)
		if err != nil {
			emitError(*jsonProgress, fmt.Sprintf("download %s: %s", track.Name, err))
			continue
		}

		if !resp.Success {
			emitError(*jsonProgress, fmt.Sprintf("download %s: %s", track.Name, resp.Error))
			continue
		}

		emitProgress(*jsonProgress, ProgressEvent{
			Type:   "metadata",
			Artist: track.Artist,
			Album:  metadata.Title,
			Title:  track.Name,
			ISRC:   track.ISRC,
		})
	}

	emitComplete(*jsonProgress, ProgressEvent{
		Type:       "complete",
		OutputPath: *outputDir,
		Artist:     metadata.Artist,
		Album:      metadata.Title,
	})
}

// Re-export types from the main package for the CLI
type App struct {
	ctx context.Context
}

type SpotifyMetadataRequest struct {
	URL       string  `json:"url"`
	Batch     bool    `json:"batch"`
	Delay     float64 `json:"delay"`
	Timeout   float64 `json:"timeout"`
	Separator string  `json:"separator,omitempty"`
}

type SpotifyMetadata struct {
	Title       string        `json:"title"`
	Artist      string        `json:"artist"`
	ReleaseDate string        `json:"release_date"`
	CoverURL    string        `json:"cover_url"`
	Tracks      []SpotifyTrack `json:"tracks"`
}

type SpotifyTrack struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Artist   string `json:"artist"`
	ISRC     string `json:"isrc"`
	Duration int    `json:"duration"`
}

type DownloadRequest struct {
	Service        string `json:"service"`
	TrackName      string `json:"track_name"`
	ArtistName     string `json:"artist_name"`
	AlbumName      string `json:"album_name"`
	AlbumArtist    string `json:"album_artist"`
	ReleaseDate    string `json:"release_date"`
	CoverURL       string `json:"cover_url"`
	OutputDir      string `json:"output_dir"`
	AudioFormat    string `json:"audio_format"`
	FilenameFormat string `json:"filename_format"`
	SpotifyID      string `json:"spotify_id"`
	ISRC           string `json:"isrc"`
	Duration       int    `json:"duration"`
	Position       int    `json:"position"`
	SaveCover      bool   `json:"save_cover"`
	EmbedLyrics    bool   `json:"embed_lyrics"`
	AllowFallback  bool   `json:"allow_fallback"`
}

type DownloadResponse struct {
	Success       bool   `json:"success"`
	Message       string `json:"message"`
	File          string `json:"file,omitempty"`
	Error         string `json:"error,omitempty"`
	AlreadyExists bool   `json:"already_exists,omitempty"`
	Cancelled     bool   `json:"cancelled,omitempty"`
}

type ProgressEvent struct {
	Type        string  `json:"type"`
	Track       string  `json:"track,omitempty"`
	Title       string  `json:"title,omitempty"`
	Artist      string  `json:"artist,omitempty"`
	Album       string  `json:"album,omitempty"`
	Percent     float64 `json:"percent,omitempty"`
	Speed       string  `json:"speed,omitempty"`
	OutputPath  string  `json:"path,omitempty"`
	Size        int64   `json:"size,omitempty"`
	ISRC        string  `json:"isrc,omitempty"`
	ErrorMessage string `json:"message,omitempty"`
}

func emitProgress(jsonOut bool, evt ProgressEvent) {
	if !jsonOut {
		return
	}
	json.NewEncoder(os.Stdout).Encode(evt)
}

func emitError(jsonOut bool, msg string) {
	if !jsonOut {
		fmt.Fprintln(os.Stderr, "error:", msg)
		return
	}
	json.NewEncoder(os.Stdout).Encode(ProgressEvent{
		Type:         "error",
		ErrorMessage: msg,
	})
}

func emitComplete(jsonOut bool, evt ProgressEvent) {
	if !jsonOut {
		fmt.Println("Download complete:", evt.OutputPath)
		return
	}
	json.NewEncoder(os.Stdout).Encode(evt)
}

func (a *App) GetSpotifyMetadata(req SpotifyMetadataRequest) (string, error) {
	// Reuse the core SpotiFLAC metadata fetching
	return backend.GetSpotifyMetadataJSON(req.URL, req.Batch, time.Duration(req.Delay)*time.Second)
}

func (a *App) DownloadTrack(req DownloadRequest) (DownloadResponse, error) {
	// Build the downloader based on service
	// The actual download is handled by SpotiFLAC's per-service backends
	outputPath, err := backend.DownloadTrack(req.Service, req.TrackName, req.ArtistName, req.AlbumName,
		req.AlbumArtist, req.ReleaseDate, req.CoverURL, req.OutputDir, req.FilenameFormat,
		req.SpotifyID, req.ISRC, req.Duration, req.Position, req.SaveCover, req.EmbedLyrics,
		req.AllowFallback)
	if err != nil {
		return DownloadResponse{Success: false, Error: err.Error()}, nil
	}
	return DownloadResponse{Success: true, File: outputPath}, nil
}

func searchSpotify(query string) ([]map[string]interface{}, error) {
	results, err := backend.SearchSpotify(query, 20)
	if err != nil {
		return nil, err
	}
	var out []map[string]interface{}
	for _, r := range results {
		out = append(out, map[string]interface{}{
			"artist":      r.Artist,
			"album":       r.Album,
			"title":       r.Title,
			"spotify_url": r.URL,
			"cover_url":   r.CoverURL,
			"genre":       r.Genre,
			"year":        r.Year,
			"track_count": r.TrackCount,
		})
	}
	return out, nil
}
