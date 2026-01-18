package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Config holds all service configurations
type Config struct {
	JellyseerrURL    string
	JellyseerrAPIKey string
	SonarrURL        string
	SonarrAPIKey     string
	RadarrURL        string
	RadarrAPIKey     string
}

var config Config

func main() {
	// Load config from environment
	config = Config{
		JellyseerrURL:    getEnv("JELLYSEERR_URL", "http://localhost:5055"),
		JellyseerrAPIKey: os.Getenv("JELLYSEERR_API_KEY"),
		SonarrURL:        getEnv("SONARR_URL", "http://localhost:8989"),
		SonarrAPIKey:     os.Getenv("SONARR_API_KEY"),
		RadarrURL:        getEnv("RADARR_URL", "http://localhost:7878"),
		RadarrAPIKey:     os.Getenv("RADARR_API_KEY"),
	}

	s := server.NewMCPServer(
		"ultimarr",
		"1.0.0",
		server.WithToolCapabilities(false),
	)

	// Register Jellyseerr tools
	registerJellyseerrTools(s)

	// Register Sonarr tools
	registerSonarrTools(s)

	// Register Radarr tools
	registerRadarrTools(s)

	// Start server
	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// ============================================================================
// HTTP Client Helpers
// ============================================================================

func doRequest(method, urlStr string, headers map[string]string, body io.Reader) ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequest(method, urlStr, body)
	if err != nil {
		return nil, err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(data[:min(200, len(data))]))
	}

	return data, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ============================================================================
// Jellyseerr
// ============================================================================

func jellyseerrRequest(method, endpoint string, body io.Reader) ([]byte, error) {
	headers := map[string]string{
		"X-Api-Key":    config.JellyseerrAPIKey,
		"Content-Type": "application/json",
	}
	return doRequest(method, config.JellyseerrURL+"/api/v1"+endpoint, headers, body)
}

func registerJellyseerrTools(s *server.MCPServer) {
	// Search
	s.AddTool(
		mcp.NewTool("jellyseerr_search",
			mcp.WithDescription("Search for movies and TV shows on Jellyseerr"),
			mcp.WithString("query", mcp.Required(), mcp.Description("Search query")),
		),
		handleJellyseerrSearch,
	)

	// Request Media
	s.AddTool(
		mcp.NewTool("jellyseerr_request",
			mcp.WithDescription("Request a movie or TV show on Jellyseerr"),
			mcp.WithNumber("tmdb_id", mcp.Required(), mcp.Description("TMDB ID of the media")),
			mcp.WithString("media_type", mcp.Required(), mcp.Description("Type: 'movie' or 'tv'")),
		),
		handleJellyseerrRequest,
	)

	// List Requests
	s.AddTool(
		mcp.NewTool("jellyseerr_list_requests",
			mcp.WithDescription("List media requests on Jellyseerr"),
			mcp.WithNumber("limit", mcp.Description("Number of requests to return (default 20)")),
		),
		handleJellyseerrListRequests,
	)
}

func handleJellyseerrSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	query := args["query"].(string)

	data, err := jellyseerrRequest("GET", "/search?query="+url.QueryEscape(query), nil)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var result map[string]interface{}
	json.Unmarshal(data, &result)

	results, _ := result["results"].([]interface{})
	var lines []string
	lines = append(lines, fmt.Sprintf("Found %d results:\n", len(results)))

	for i, r := range results {
		if i >= 15 {
			break
		}
		item := r.(map[string]interface{})
		mediaType := item["mediaType"].(string)
		name := ""
		if n, ok := item["name"].(string); ok {
			name = n
		} else if t, ok := item["title"].(string); ok {
			name = t
		}
		year := ""
		if d, ok := item["firstAirDate"].(string); ok && len(d) >= 4 {
			year = d[:4]
		} else if d, ok := item["releaseDate"].(string); ok && len(d) >= 4 {
			year = d[:4]
		}
		id := int(item["id"].(float64))

		status := ""
		if mi, ok := item["mediaInfo"].(map[string]interface{}); ok {
			if s, ok := mi["status"].(float64); ok {
				statusMap := map[int]string{2: "[Pending]", 3: "[Processing]", 4: "[Available]", 5: "[Partial]"}
				status = statusMap[int(s)]
			}
		}

		lines = append(lines, fmt.Sprintf("  [%s] %s (%s) - TMDB: %d %s", strings.ToUpper(mediaType), name, year, id, status))
	}

	return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
}

func handleJellyseerrRequest(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	tmdbID := int(args["tmdb_id"].(float64))
	mediaType := args["media_type"].(string)

	payload := map[string]interface{}{
		"mediaType": mediaType,
		"mediaId":   tmdbID,
	}
	if mediaType == "tv" {
		payload["seasons"] = "all"
	}

	body, _ := json.Marshal(payload)
	data, err := jellyseerrRequest("POST", "/request", strings.NewReader(string(body)))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var result map[string]interface{}
	json.Unmarshal(data, &result)

	if id, ok := result["id"]; ok {
		return mcp.NewToolResultText(fmt.Sprintf("Request created successfully. Request ID: %v", id)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Response: %s", string(data))), nil
}

func handleJellyseerrListRequests(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	limit := 20
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	data, err := jellyseerrRequest("GET", fmt.Sprintf("/request?take=%d", limit), nil)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var result map[string]interface{}
	json.Unmarshal(data, &result)

	results, _ := result["results"].([]interface{})
	var lines []string
	lines = append(lines, fmt.Sprintf("Requests (%d):\n", len(results)))

	statusMap := map[int]string{1: "Pending", 2: "Approved", 3: "Declined"}

	for _, r := range results {
		item := r.(map[string]interface{})
		reqID := int(item["id"].(float64))
		status := statusMap[int(item["status"].(float64))]
		media := item["media"].(map[string]interface{})
		mediaType := media["mediaType"].(string)
		tmdbID := int(media["tmdbId"].(float64))

		user := "Unknown"
		if rb, ok := item["requestedBy"].(map[string]interface{}); ok {
			if dn, ok := rb["displayName"].(string); ok {
				user = dn
			}
		}

		lines = append(lines, fmt.Sprintf("  #%d [%s] %s (TMDB: %d) - by %s", reqID, status, mediaType, tmdbID, user))
	}

	return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
}

// ============================================================================
// Sonarr
// ============================================================================

func sonarrRequest(method, endpoint string, body io.Reader) ([]byte, error) {
	headers := map[string]string{
		"X-Api-Key":    config.SonarrAPIKey,
		"Content-Type": "application/json",
	}
	return doRequest(method, config.SonarrURL+"/api/v3"+endpoint, headers, body)
}

func registerSonarrTools(s *server.MCPServer) {
	// List Series
	s.AddTool(
		mcp.NewTool("sonarr_list_series",
			mcp.WithDescription("List all TV series in Sonarr"),
		),
		handleSonarrListSeries,
	)

	// Get Series
	s.AddTool(
		mcp.NewTool("sonarr_get_series",
			mcp.WithDescription("Get details for a specific series in Sonarr"),
			mcp.WithNumber("series_id", mcp.Required(), mcp.Description("Sonarr series ID")),
		),
		handleSonarrGetSeries,
	)

	// Search Series (trigger search for releases)
	s.AddTool(
		mcp.NewTool("sonarr_search_series",
			mcp.WithDescription("Trigger a search for releases for a series in Sonarr"),
			mcp.WithNumber("series_id", mcp.Required(), mcp.Description("Sonarr series ID")),
		),
		handleSonarrSearchSeries,
	)

	// Interactive Search (get available releases)
	s.AddTool(
		mcp.NewTool("sonarr_get_releases",
			mcp.WithDescription("Get available releases for a series (interactive search)"),
			mcp.WithNumber("series_id", mcp.Required(), mcp.Description("Sonarr series ID")),
			mcp.WithNumber("season", mcp.Description("Season number (optional, omit for all)")),
		),
		handleSonarrGetReleases,
	)

	// Download Release
	s.AddTool(
		mcp.NewTool("sonarr_download_release",
			mcp.WithDescription("Download a specific release by GUID"),
			mcp.WithString("guid", mcp.Required(), mcp.Description("Release GUID from sonarr_get_releases")),
			mcp.WithNumber("indexer_id", mcp.Required(), mcp.Description("Indexer ID from the release")),
			mcp.WithNumber("series_id", mcp.Required(), mcp.Description("Sonarr series ID")),
		),
		handleSonarrDownloadRelease,
	)

	// Queue
	s.AddTool(
		mcp.NewTool("sonarr_queue",
			mcp.WithDescription("Get current download queue in Sonarr"),
		),
		handleSonarrQueue,
	)
}

func handleSonarrListSeries(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	data, err := sonarrRequest("GET", "/series", nil)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var series []map[string]interface{}
	json.Unmarshal(data, &series)

	var lines []string
	lines = append(lines, fmt.Sprintf("Series in Sonarr (%d):\n", len(series)))

	for _, s := range series {
		id := int(s["id"].(float64))
		title := s["title"].(string)
		year := 0
		if y, ok := s["year"].(float64); ok {
			year = int(y)
		}
		status := s["status"].(string)
		monitored := s["monitored"].(bool)

		monStr := ""
		if !monitored {
			monStr = " [unmonitored]"
		}

		lines = append(lines, fmt.Sprintf("  [%d] %s (%d) - %s%s", id, title, year, status, monStr))
	}

	return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
}

func handleSonarrGetSeries(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	seriesID := int(args["series_id"].(float64))

	data, err := sonarrRequest("GET", fmt.Sprintf("/series/%d", seriesID), nil)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var s map[string]interface{}
	json.Unmarshal(data, &s)

	title := s["title"].(string)
	year := int(s["year"].(float64))
	status := s["status"].(string)
	path := s["path"].(string)
	monitored := s["monitored"].(bool)

	episodeCount := 0
	episodeFileCount := 0
	if stats, ok := s["statistics"].(map[string]interface{}); ok {
		if ec, ok := stats["episodeCount"].(float64); ok {
			episodeCount = int(ec)
		}
		if efc, ok := stats["episodeFileCount"].(float64); ok {
			episodeFileCount = int(efc)
		}
	}

	info := fmt.Sprintf(`**%s** (%d)
ID: %d
Status: %s
Monitored: %v
Path: %s
Episodes: %d/%d downloaded`, title, year, seriesID, status, monitored, path, episodeFileCount, episodeCount)

	return mcp.NewToolResultText(info), nil
}

func handleSonarrSearchSeries(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	seriesID := int(args["series_id"].(float64))

	payload := map[string]interface{}{
		"name":     "SeriesSearch",
		"seriesId": seriesID,
	}
	body, _ := json.Marshal(payload)

	data, err := sonarrRequest("POST", "/command", strings.NewReader(string(body)))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var result map[string]interface{}
	json.Unmarshal(data, &result)

	return mcp.NewToolResultText(fmt.Sprintf("Search triggered. Command ID: %v", result["id"])), nil
}

func handleSonarrGetReleases(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	seriesID := int(args["series_id"].(float64))

	endpoint := fmt.Sprintf("/release?seriesId=%d", seriesID)
	if season, ok := args["season"].(float64); ok {
		endpoint += fmt.Sprintf("&seasonNumber=%d", int(season))
	}

	data, err := sonarrRequest("GET", endpoint, nil)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var releases []map[string]interface{}
	json.Unmarshal(data, &releases)

	var lines []string
	lines = append(lines, fmt.Sprintf("Available releases (%d):\n", len(releases)))

	for i, r := range releases {
		if i >= 20 {
			lines = append(lines, fmt.Sprintf("\n  ... and %d more", len(releases)-20))
			break
		}
		title := r["title"].(string)
		size := int64(r["size"].(float64))
		sizeMB := size / 1024 / 1024
		seeders := 0
		if s, ok := r["seeders"].(float64); ok {
			seeders = int(s)
		}
		guid := r["guid"].(string)
		indexerID := int(r["indexerId"].(float64))
		indexer := r["indexer"].(string)

		lines = append(lines, fmt.Sprintf("  [%d seeders] %s (%dMB) - %s\n    GUID: %s | Indexer: %d", seeders, title[:min(60, len(title))], sizeMB, indexer, guid, indexerID))
	}

	return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
}

func handleSonarrDownloadRelease(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	guid := args["guid"].(string)
	indexerID := int(args["indexer_id"].(float64))
	seriesID := int(args["series_id"].(float64))

	payload := map[string]interface{}{
		"guid":      guid,
		"indexerId": indexerID,
		"seriesId":  seriesID,
	}
	body, _ := json.Marshal(payload)

	_, err := sonarrRequest("POST", "/release", strings.NewReader(string(body)))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText("Download started successfully"), nil
}

func handleSonarrQueue(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	data, err := sonarrRequest("GET", "/queue", nil)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var result map[string]interface{}
	json.Unmarshal(data, &result)

	records, _ := result["records"].([]interface{})

	var lines []string
	lines = append(lines, fmt.Sprintf("Download Queue (%d items):\n", len(records)))

	for _, r := range records {
		item := r.(map[string]interface{})
		title := item["title"].(string)
		status := item["status"].(string)
		sizeleft := int64(0)
		if sl, ok := item["sizeleft"].(float64); ok {
			sizeleft = int64(sl) / 1024 / 1024
		}

		lines = append(lines, fmt.Sprintf("  %s - %s (%dMB left)", title, status, sizeleft))
	}

	if len(records) == 0 {
		lines = append(lines, "  (empty)")
	}

	return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
}

// ============================================================================
// Radarr
// ============================================================================

func radarrRequest(method, endpoint string, body io.Reader) ([]byte, error) {
	headers := map[string]string{
		"X-Api-Key":    config.RadarrAPIKey,
		"Content-Type": "application/json",
	}
	return doRequest(method, config.RadarrURL+"/api/v3"+endpoint, headers, body)
}

func registerRadarrTools(s *server.MCPServer) {
	// List Movies
	s.AddTool(
		mcp.NewTool("radarr_list_movies",
			mcp.WithDescription("List all movies in Radarr"),
		),
		handleRadarrListMovies,
	)

	// Get Movie
	s.AddTool(
		mcp.NewTool("radarr_get_movie",
			mcp.WithDescription("Get details for a specific movie in Radarr"),
			mcp.WithNumber("movie_id", mcp.Required(), mcp.Description("Radarr movie ID")),
		),
		handleRadarrGetMovie,
	)

	// Search Movie
	s.AddTool(
		mcp.NewTool("radarr_search_movie",
			mcp.WithDescription("Trigger a search for releases for a movie in Radarr"),
			mcp.WithNumber("movie_id", mcp.Required(), mcp.Description("Radarr movie ID")),
		),
		handleRadarrSearchMovie,
	)

	// Get Releases
	s.AddTool(
		mcp.NewTool("radarr_get_releases",
			mcp.WithDescription("Get available releases for a movie (interactive search)"),
			mcp.WithNumber("movie_id", mcp.Required(), mcp.Description("Radarr movie ID")),
		),
		handleRadarrGetReleases,
	)

	// Download Release
	s.AddTool(
		mcp.NewTool("radarr_download_release",
			mcp.WithDescription("Download a specific release by GUID"),
			mcp.WithString("guid", mcp.Required(), mcp.Description("Release GUID from radarr_get_releases")),
			mcp.WithNumber("indexer_id", mcp.Required(), mcp.Description("Indexer ID from the release")),
			mcp.WithNumber("movie_id", mcp.Required(), mcp.Description("Radarr movie ID")),
		),
		handleRadarrDownloadRelease,
	)

	// Queue
	s.AddTool(
		mcp.NewTool("radarr_queue",
			mcp.WithDescription("Get current download queue in Radarr"),
		),
		handleRadarrQueue,
	)
}

func handleRadarrListMovies(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	data, err := radarrRequest("GET", "/movie", nil)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var movies []map[string]interface{}
	json.Unmarshal(data, &movies)

	var lines []string
	lines = append(lines, fmt.Sprintf("Movies in Radarr (%d):\n", len(movies)))

	for _, m := range movies {
		id := int(m["id"].(float64))
		title := m["title"].(string)
		year := 0
		if y, ok := m["year"].(float64); ok {
			year = int(y)
		}
		hasFile := m["hasFile"].(bool)
		monitored := m["monitored"].(bool)

		status := "missing"
		if hasFile {
			status = "downloaded"
		}
		if !monitored {
			status += " [unmonitored]"
		}

		lines = append(lines, fmt.Sprintf("  [%d] %s (%d) - %s", id, title, year, status))
	}

	return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
}

func handleRadarrGetMovie(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	movieID := int(args["movie_id"].(float64))

	data, err := radarrRequest("GET", fmt.Sprintf("/movie/%d", movieID), nil)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var m map[string]interface{}
	json.Unmarshal(data, &m)

	title := m["title"].(string)
	year := int(m["year"].(float64))
	hasFile := m["hasFile"].(bool)
	monitored := m["monitored"].(bool)
	path := ""
	if p, ok := m["path"].(string); ok {
		path = p
	}

	status := "Missing"
	if hasFile {
		status = "Downloaded"
	}

	info := fmt.Sprintf(`**%s** (%d)
ID: %d
Status: %s
Monitored: %v
Path: %s`, title, year, movieID, status, monitored, path)

	return mcp.NewToolResultText(info), nil
}

func handleRadarrSearchMovie(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	movieID := int(args["movie_id"].(float64))

	payload := map[string]interface{}{
		"name":     "MoviesSearch",
		"movieIds": []int{movieID},
	}
	body, _ := json.Marshal(payload)

	data, err := radarrRequest("POST", "/command", strings.NewReader(string(body)))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var result map[string]interface{}
	json.Unmarshal(data, &result)

	return mcp.NewToolResultText(fmt.Sprintf("Search triggered. Command ID: %v", result["id"])), nil
}

func handleRadarrGetReleases(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	movieID := int(args["movie_id"].(float64))

	data, err := radarrRequest("GET", fmt.Sprintf("/release?movieId=%d", movieID), nil)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var releases []map[string]interface{}
	json.Unmarshal(data, &releases)

	var lines []string
	lines = append(lines, fmt.Sprintf("Available releases (%d):\n", len(releases)))

	for i, r := range releases {
		if i >= 20 {
			lines = append(lines, fmt.Sprintf("\n  ... and %d more", len(releases)-20))
			break
		}
		title := r["title"].(string)
		size := int64(r["size"].(float64))
		sizeMB := size / 1024 / 1024
		seeders := 0
		if s, ok := r["seeders"].(float64); ok {
			seeders = int(s)
		}
		guid := r["guid"].(string)
		indexerID := int(r["indexerId"].(float64))
		indexer := r["indexer"].(string)

		lines = append(lines, fmt.Sprintf("  [%d seeders] %s (%dMB) - %s\n    GUID: %s | Indexer: %d", seeders, title[:min(60, len(title))], sizeMB, indexer, guid, indexerID))
	}

	return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
}

func handleRadarrDownloadRelease(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	guid := args["guid"].(string)
	indexerID := int(args["indexer_id"].(float64))
	movieID := int(args["movie_id"].(float64))

	payload := map[string]interface{}{
		"guid":      guid,
		"indexerId": indexerID,
		"movieId":   movieID,
	}
	body, _ := json.Marshal(payload)

	_, err := radarrRequest("POST", "/release", strings.NewReader(string(body)))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText("Download started successfully"), nil
}

func handleRadarrQueue(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	data, err := radarrRequest("GET", "/queue", nil)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var result map[string]interface{}
	json.Unmarshal(data, &result)

	records, _ := result["records"].([]interface{})

	var lines []string
	lines = append(lines, fmt.Sprintf("Download Queue (%d items):\n", len(records)))

	for _, r := range records {
		item := r.(map[string]interface{})
		title := item["title"].(string)
		status := item["status"].(string)
		sizeleft := int64(0)
		if sl, ok := item["sizeleft"].(float64); ok {
			sizeleft = int64(sl) / 1024 / 1024
		}

		lines = append(lines, fmt.Sprintf("  %s - %s (%dMB left)", title, status, sizeleft))
	}

	if len(records) == 0 {
		lines = append(lines, "  (empty)")
	}

	return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
}
