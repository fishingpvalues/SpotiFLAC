# Maintaining this fork

The upstream project (github.com/spotbye/SpotiFLAC) has been deleted. This
fork is now the canonical source.

- Primary remote: Forgejo, `daniel/SpotiFLAC` on the private potatostack forge.
- GitHub (`fishingpvalues/SpotiFLAC`) is a secondary copy, kept so that
  spotiflac-lidarr-proxy image builds on GitHub Actions can still clone it.
  Push to both remotes.
- spotiflac-lidarr-proxy pins this repo by commit (`SPOTIFLAC_COMMIT`).

## Runtime overrides for the community hosts

The community download hosts are built in (encrypted). If one moves or dies,
point at a replacement without rebuilding:

| Variable | Overrides |
|---|---|
| `SPOTIFLAC_TIDAL_COMMUNITY_URL` | Tidal community base URL |
| `SPOTIFLAC_QOBUZ_COMMUNITY_URL` | Qobuz community base URL |
| `SPOTIFLAC_AMAZON_COMMUNITY_URL` | Amazon community base URL |
| `SPOTIFLAC_VERIFY_URL` | verification relay base URL |

Values are base URLs without a path or trailing slash (`/api/dl` and
`/health` are appended).

## Build check

    go build -tags headless ./... && go vet -tags headless ./...
