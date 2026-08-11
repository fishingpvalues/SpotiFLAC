//go:build headless

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
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

	// The Wails GUI opens the community verification challenge in the
	// user's own visible browser (see app.go's startup(): runtime.
	// BrowserOpenURL) and its callback listens on 127.0.0.1, which only
	// works because that browser runs on the same machine. Headless mode
	// has neither, so it used to fail immediately ("browser integration is
	// not ready"). If SPOTIFLAC_VERIFY_RELAY_URL is set, this instead
	// rewrites the challenge's own "cb" redirect target to point there and
	// emits the resulting URL on stdout for a caller to relay to a real
	// person's real browser - the actual verification challenge itself is
	// untouched and still requires a real human to complete it; this only
	// solves the network-reachability gap between a loopback-bound
	// listener on a headless host and a browser on a different machine.
	// With no relay URL configured, this is a no-op and the CLI behaves
	// exactly as before (verification simply cannot complete headlessly).
	backend.SetCommunityVerificationHandlers(openCommunityVerificationRelay, func() {})

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
				"entity":      r.Type,
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
			// An album SearchResult carries its own title in Name and leaves
			// AlbumName empty (only track results fill AlbumName, with their
			// containing album). Emitting r.AlbumName here therefore published
			// every album hit with "album": "", and consumers that key on the
			// album field could not tell an album apart from a nameless track:
			// spotiflac-lidarr-proxy dropped all of them, which left Lidarr
			// with nothing but single tracks to grab and nothing it could
			// import (it needs >= 80% of an album's tracks).
			emitJSON(map[string]interface{}{
				"type":        "search_result",
				"entity":      r.Type,
				"name":        r.Name,
				"artist":      r.Artists,
				"album":       r.Name,
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

	// Parse metadata. GetFilteredSpotifyData's actual JSON shape depends on
	// the URL type - previously this only checked a "tracks" array (a key
	// that doesn't exist in any of these responses) and silently fell
	// through to a near-empty DownloadRequest for every URL, guaranteed to
	// fail regardless of service (album/playlist real key is "track_list",
	// singular track is nested under "track", not "tracks"). See
	// backend.AlbumResponsePayload / PlaylistResponsePayload / TrackResponse.
	var metadata struct {
		AlbumInfo *struct {
			Name        string `json:"name"`
			Artists     string `json:"artists"`
			ReleaseDate string `json:"release_date"`
			Images      string `json:"images"`
		} `json:"album_info"`
		PlaylistInfo *struct {
			Cover string `json:"cover"`
			Owner struct {
				DisplayName string `json:"display_name"`
			} `json:"owner"`
		} `json:"playlist_info"`
		TrackList []cliTrackMeta `json:"track_list"`
		Track     *cliTrackMeta  `json:"track"`
	}
	if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
		emitJSON(map[string]interface{}{
			"type":    "error",
			"message": fmt.Sprintf("unrecognized metadata response: %s", err),
		})
		os.Exit(1)
	}

	if metadata.Track != nil {
		// Single track URL: backend returns one "track" object, not a list.
		t := *metadata.Track
		req := DownloadRequest{
			Service:            cfg.Service,
			TrackName:          t.Name,
			ArtistName:         t.Artists,
			AlbumName:          t.AlbumName,
			AlbumArtist:        t.AlbumArtist,
			ReleaseDate:        t.ReleaseDate,
			CoverURL:           t.Images,
			OutputDir:          cfg.OutputDir,
			AudioFormat:        cfg.Quality,
			SpotifyID:          t.SpotifyID,
			Duration:           t.DurationMS,
			Position:           1,
			SpotifyTrackNumber: t.TrackNumber,
			SpotifyDiscNumber:  t.DiscNumber,
			SpotifyTotalTracks: t.TotalTracks,
			SpotifyTotalDiscs:  t.TotalDiscs,
			UPC:                t.UPC,
			Copyright:          t.Copyright,
			Publisher:          t.Publisher,
			Composer:           t.Composer,
			SaveCover:          true,
			EmbedLyrics:        true,
			TidalAPIURL:        cfg.TidalAPIURL,
			QobuzAPIURL:        cfg.QobuzAPIURL,
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

	if len(metadata.TrackList) == 0 {
		emitJSON(map[string]interface{}{
			"type":    "error",
			"message": "metadata response had neither a single track nor a track_list",
		})
		os.Exit(1)
	}

	albumName, artistName, releaseDate, coverURL := "", "", "", ""
	switch {
	case metadata.AlbumInfo != nil:
		albumName = metadata.AlbumInfo.Name
		artistName = metadata.AlbumInfo.Artists
		releaseDate = metadata.AlbumInfo.ReleaseDate
		coverURL = metadata.AlbumInfo.Images
	case metadata.PlaylistInfo != nil:
		artistName = metadata.PlaylistInfo.Owner.DisplayName
		coverURL = metadata.PlaylistInfo.Cover
	}

	emitJSON(map[string]interface{}{
		"type":      "metadata",
		"artist":    artistName,
		"album":     albumName,
		"released":  releaseDate,
		"cover_url": coverURL,
		"tracks":    len(metadata.TrackList),
	})

	// Download each track
	for i, track := range metadata.TrackList {
		emitJSON(map[string]interface{}{
			"type":    "progress",
			"track":   fmt.Sprintf("%02d", i+1),
			"title":   track.Name,
			"percent": float64(i) / float64(len(metadata.TrackList)) * 100,
		})

		req := DownloadRequest{
			Service:            cfg.Service,
			TrackName:          track.Name,
			ArtistName:         track.Artists,
			AlbumName:          track.AlbumName,
			AlbumArtist:        track.AlbumArtist,
			ReleaseDate:        track.ReleaseDate,
			CoverURL:           track.Images,
			OutputDir:          cfg.OutputDir,
			AudioFormat:        cfg.Quality,
			SpotifyID:          track.SpotifyID,
			Duration:           track.DurationMS,
			Position:           i + 1,
			SpotifyTrackNumber: track.TrackNumber,
			SpotifyDiscNumber:  track.DiscNumber,
			SpotifyTotalTracks: track.TotalTracks,
			SpotifyTotalDiscs:  track.TotalDiscs,
			UPC:                track.UPC,
			Copyright:          track.Copyright,
			Publisher:          track.Publisher,
			Composer:           track.Composer,
			SaveCover:          true,
			EmbedLyrics:        true,
			TidalAPIURL:        cfg.TidalAPIURL,
			QobuzAPIURL:        cfg.QobuzAPIURL,
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
			"artist": track.Artists,
			"album":  track.AlbumName,
			"title":  track.Name,
			"path":   resp.File,
		})
	}

	emitJSON(map[string]interface{}{
		"type":   "complete",
		"path":   cfg.OutputDir,
		"artist": artistName,
		"album":  albumName,
	})
}

// cliTrackMeta mirrors the fields shared by backend.TrackMetadata and
// backend.AlbumTrackMetadata that the CLI actually needs to build a
// DownloadRequest. Kept separate from those exported types since neither
// is guaranteed to stay JSON-tag-identical to the other over time, and this
// package only cares about a subset of their fields.
type cliTrackMeta struct {
	SpotifyID   string `json:"spotify_id"`
	Name        string `json:"name"`
	Artists     string `json:"artists"`
	AlbumName   string `json:"album_name"`
	AlbumArtist string `json:"album_artist"`
	DurationMS  int    `json:"duration_ms"`
	Images      string `json:"images"`
	ReleaseDate string `json:"release_date"`
	TrackNumber int    `json:"track_number"`
	TotalTracks int    `json:"total_tracks"`
	DiscNumber  int    `json:"disc_number"`
	TotalDiscs  int    `json:"total_discs"`
	UPC         string `json:"upc"`
	Copyright   string `json:"copyright"`
	Publisher   string `json:"publisher"`
	Composer    string `json:"composer"`
}

// openCommunityVerificationRelay is the openBrowser handler passed to
// backend.SetCommunityVerificationHandlers. The challengeURL it receives
// already has a "cb" query param baked in pointing at
// runCommunityVerification's own 127.0.0.1 listener - see
// backend/community_session.go. Without a relay configured, this just
// emits the raw URL (useful if the CLI happens to run on the same machine
// as the person completing verification). With SPOTIFLAC_VERIFY_RELAY_URL
// set, it swaps "cb" for that relay URL (carrying the original loopback cb
// along as its own "upstream_cb" param) so a remote caller can complete the
// real challenge in their own browser and have the relay forward the
// resulting grant back to the loopback listener on this machine.
func openCommunityVerificationRelay(target string) {
	relayBase := os.Getenv("SPOTIFLAC_VERIFY_RELAY_URL")
	if relayBase == "" {
		emitJSON(map[string]interface{}{
			"type": "verification_required",
			"url":  target,
		})
		return
	}

	parsed, err := url.Parse(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "verification URL parse failed: %v\n", err)
		return
	}
	q := parsed.Query()
	upstreamCB := q.Get("cb")
	if upstreamCB == "" {
		fmt.Fprintf(os.Stderr, "verification URL missing cb param, cannot relay\n")
		emitJSON(map[string]interface{}{
			"type": "verification_required",
			"url":  target,
		})
		return
	}

	relay, err := url.Parse(relayBase)
	if err != nil {
		fmt.Fprintf(os.Stderr, "SPOTIFLAC_VERIFY_RELAY_URL is invalid: %v\n", err)
		emitJSON(map[string]interface{}{
			"type": "verification_required",
			"url":  target,
		})
		return
	}
	rq := relay.Query()
	rq.Set("upstream_cb", upstreamCB)
	relay.RawQuery = rq.Encode()

	q.Set("cb", relay.String())
	parsed.RawQuery = q.Encode()

	emitJSON(map[string]interface{}{
		"type": "verification_required",
		"url":  parsed.String(),
		// The relay's own caller needs the real upstream_cb as a distinct,
		// already-parsed field: the relay must only ever forward a grant to
		// exactly this value, never to whatever a request happens to claim
		// upstream_cb is (see internal/api/verify in spotiflac-lidarr-proxy).
		"cb": upstreamCB,
	})
}

type cliConfig struct {
	URL         string
	OutputDir   string
	Service     string
	Quality     string
	Search      string
	TidalAPIURL string
	QobuzAPIURL string
}

func loadCLIFlags() cliConfig {
	var cfg cliConfig
	flag.StringVar(&cfg.URL, "url", "", "Spotify URL to download (track/album/playlist)")
	flag.StringVar(&cfg.OutputDir, "output-dir", ".", "Output directory")
	flag.StringVar(&cfg.Service, "service", "tidal", "Download service: tidal, qobuz, amazon, deezer")
	flag.StringVar(&cfg.Quality, "quality", "LOSSLESS", "Quality: LOSSLESS, HIRES_LOSSLESS")
	flag.StringVar(&cfg.Search, "search", "", "Search Spotify instead of downloading")
	// A custom Tidal/Qobuz API instance (self-hosted or a known public one)
	// is tried first and, if it works, skips the community tier entirely -
	// see backend/tidal.go's GetDownloadURL and backend/qobuz.go's
	// GetDownloadURL, both already support this; the CLI just never
	// exposed flags for it. Falls back to the community tier (and its
	// one-time verification requirement) only if unset or unreachable.
	flag.StringVar(&cfg.TidalAPIURL, "tidal-api-url", "", "Custom Tidal API instance URL (bypasses the community tier if set)")
	flag.StringVar(&cfg.QobuzAPIURL, "qobuz-api-url", "", "Custom Qobuz API instance URL (bypasses the community tier if set)")
	flag.Parse()
	return cfg
}

func emitJSON(v interface{}) {
	json.NewEncoder(os.Stdout).Encode(v)
}
