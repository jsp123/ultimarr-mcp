# ultimarr-mcp

MCP (Model Context Protocol) server for the *arr stack - control Jellyseerr, Sonarr, and Radarr from Claude.

## Installation

Download the binary from [releases](https://github.com/jsp123/ultimarr-mcp/releases) or build from source:

```bash
go build -o ultimarr-mcp .
```

## Configuration

Requires 6 environment variables (3 services × 2 each):

| Variable | Description | Default |
|----------|-------------|---------|
| `JELLYSEERR_URL` | Jellyseerr base URL | `http://localhost:5055` |
| `JELLYSEERR_API_KEY` | Jellyseerr API key | (required) |
| `SONARR_URL` | Sonarr base URL | `http://localhost:8989` |
| `SONARR_API_KEY` | Sonarr API key | (required) |
| `RADARR_URL` | Radarr base URL | `http://localhost:7878` |
| `RADARR_API_KEY` | Radarr API key | (required) |

### Finding your API keys

- **Jellyseerr**: Settings → General → API Key
- **Sonarr**: Settings → General → API Key
- **Radarr**: Settings → General → API Key

## Claude Code Setup

Add to `~/.claude/settings.json`:

```json
{
  "mcpServers": {
    "ultimarr": {
      "command": "/path/to/ultimarr-mcp",
      "args": [],
      "env": {
        "JELLYSEERR_URL": "http://localhost:5055",
        "JELLYSEERR_API_KEY": "your-jellyseerr-key",
        "SONARR_URL": "http://localhost:8989",
        "SONARR_API_KEY": "your-sonarr-key",
        "RADARR_URL": "http://localhost:7878",
        "RADARR_API_KEY": "your-radarr-key"
      }
    }
  }
}
```

## Available Tools

### Jellyseerr (3 tools)
| Tool | Description |
|------|-------------|
| `jellyseerr_search` | Search for movies and TV shows |
| `jellyseerr_request` | Request a movie or TV show |
| `jellyseerr_list_requests` | List media requests |

### Sonarr (6 tools)
| Tool | Description |
|------|-------------|
| `sonarr_list_series` | List all TV series |
| `sonarr_get_series` | Get details for a specific series |
| `sonarr_search_series` | Trigger a search for releases |
| `sonarr_get_releases` | Get available releases (interactive search) |
| `sonarr_download_release` | Download a specific release |
| `sonarr_queue` | Get current download queue |

### Radarr (6 tools)
| Tool | Description |
|------|-------------|
| `radarr_list_movies` | List all movies |
| `radarr_get_movie` | Get details for a specific movie |
| `radarr_search_movie` | Trigger a search for releases |
| `radarr_get_releases` | Get available releases (interactive search) |
| `radarr_download_release` | Download a specific release |
| `radarr_queue` | Get current download queue |

## Usage Examples

Once configured, you can use natural language with Claude:

- "Search for Breaking Bad on Jellyseerr"
- "Show me all my TV series in Sonarr"
- "What's in the Radarr download queue?"
- "Find releases for series ID 42 and download the one with the most seeders"

## License

MIT
