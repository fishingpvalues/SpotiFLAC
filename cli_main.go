//go:build headless

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/fishingpvalues/SpotiFLAC/backend"
)

func main() {
	cfg := loadCLIFlags()

	// Initialize backend without Wails runtime
	_ = backend.InitHistoryDB("SpotiFLAC-CLI")
	_ = backend.InitISRCCacheDB()
	_ = backend.CleanupLegacyTidalPublicAPIState()
	_ = backend.SanitizePersistedConfigSettings()

	app := NewApp()
	app.SetContext(context.Background())

	// Search mode
	if cfg.Search != "" {
		results, err := backend.SearchSpotify(context.Background(), cfg.Search, 20)
		if err != nil {
			fmt.Fprintf(os.Stderr, "search error: %s\n", err)
			os.Exit(1)
		}
		for _, r := range results.Tracks {
			emitJSON(map[string]interface{}{
				"type":        "search_result",
				"name":        r.Name,
				"artist":      r.Artists,
				"album":       r.AlbumName,
				"spotify_url": r.ExternalURL,
				"cover_url":   r.Images,
				"year":        r.ReleaseDate,
				"track_count": r.TotalTracks,
			})
		}
		for _, r := range results.Albums {
			emitJSON(map[string]interface{}{
				"type":        "search_result",
				"name":        r.Name,
				"artist":      r.Artists,
				"album":       r.AlbumName,
				"spotify_url": r.ExternalURL,
				"cover_url":   r.Images,
				"year":        r.ReleaseDate,
				"track_count": r.TotalTracks,
			})
		}
		return
	}

	if cfg.URL == "" {
		fmt.Fprintln(os.Stderr, "error: --url is required (spotify track/album/playlist URL)")
		os.Exit(1)
	}

	emitJSON(map[string]interface{}{
		"type":    "status",
		"message": "fetching metadata",
		"url":     cfg.URL,
	})

	// Fetch Spotify metadata
	metadataJSON, err := app.GetSpotifyMetadata(SpotifyMetadataRequest{
		URL:   cfg.URL,
		Batch: true,
	})
	if err != nil {
		emitJSON(map[string]interface{}{
			"type":    "error",
			"message": fmt.Sprintf("metadata fetch failed: %s", err),
		})
		os.Exit(1)
	}

	// Parse metadata to extract track list
	var metadata struct {
		Tracks []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Artist   string `json:"artist"`
			ISRC     string `json:"isrc"`
			Duration int    `json:"duration"`
		} `json:"tracks"`
		Title       string `json:"title"`
		Artist      string `json:"artist"`
		ReleaseDate string `json:"release_date"`
		CoverURL    string `json:"cover_url"`
	}
	if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil || len(metadata.Tracks) == 0 {
		// Try single download — metadata might be a single track
		req := DownloadRequest{
			Service:     cfg.Service,
			OutputDir:   cfg.OutputDir,
			AudioFormat: cfg.Quality,
		}
		resp, err := app.DownloadTrack(req)
		if err != nil || !resp.Success {
			errMsg := "unknown error"
			if resp.Error != "" {
				errMsg = resp.Error
			} else if err != nil {
				errMsg = err.Error()
			}
			emitJSON(map[string]interface{}{
				"type":    "error",
				"message": errMsg,
			})
			os.Exit(1)
		}
		emitJSON(map[string]interface{}{
			"type": "complete",
			"path": resp.File,
		})
		return
	}

	emitJSON(map[string]interface{}{
		"type":     "metadata",
		"artist":   metadata.Artist,
		"album":    metadata.Title,
		"released": metadata.ReleaseDate,
		"tracks":   len(metadata.Tracks),
	})

	// Download each track
	for i, track := range metadata.Tracks {
		emitJSON(map[string]interface{}{
			"type":    "progress",
			"track":   fmt.Sprintf("%02d", i+1),
			"title":   track.Name,
			"percent": float64(i) / float64(len(metadata.Tracks)) * 100,
		})

		req := DownloadRequest{
			Service:      cfg.Service,
			TrackName:    track.Name,
			ArtistName:   track.Artist,
			AlbumName:    metadata.Title,
			AlbumArtist:  metadata.Artist,
			ReleaseDate:  metadata.ReleaseDate,
			CoverURL:     metadata.CoverURL,
			OutputDir:    cfg.OutputDir,
			SpotifyID:    track.ID,
			ISRC:         track.ISRC,
			Duration:     track.Duration,
			Position:     i + 1,
			SaveCover:    true,
			EmbedLyrics:  true,
		}

		resp, err := app.DownloadTrack(req)
		if err != nil || !resp.Success {
			errMsg := "unknown error"
			if resp.Error != "" {
				errMsg = resp.Error
			} else if err != nil {
				errMsg = err.Error()
			}
			emitJSON(map[string]interface{}{
				"type":    "error",
				"message": fmt.Sprintf("track %s: %s", track.Name, errMsg),
			})
			continue
		}

		emitJSON(map[string]interface{}{
			"type":   "track_done",
			"artist": track.Artist,
			"album":  metadata.Title,
			"title":  track.Name,
			"isrc":   track.ISRC,
			"path":   resp.File,
		})
	}

	emitJSON(map[string]interface{}{
		"type":   "complete",
		"path":   cfg.OutputDir,
		"artist": metadata.Artist,
		"album":  metadata.Title,
	})
}

type cliConfig struct {
	URL       string
	OutputDir string
	Service   string
	Quality   string
	Search    string
}

func loadCLIFlags() cliConfig {
	var cfg cliConfig
	flag.StringVar(&cfg.URL, "url", "", "Spotify URL to download (track/album/playlist)")
	flag.StringVar(&cfg.OutputDir, "output-dir", ".", "Output directory")
	flag.StringVar(&cfg.Service, "service", "tidal", "Download service: tidal, qobuz, amazon, deezer")
	flag.StringVar(&cfg.Quality, "quality", "LOSSLESS", "Quality: LOSSLESS, HIRES_LOSSLESS")
	flag.StringVar(&cfg.Search, "search", "", "Search Spotify instead of downloading")
	flag.Parse()
	return cfg
}

func emitJSON(v interface{}) {
	json.NewEncoder(os.Stdout).Encode(v)
}
