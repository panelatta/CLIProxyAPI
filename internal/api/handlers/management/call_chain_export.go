package management

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	callChainExportVersion      = 1
	defaultCallChainExportLimit = 200
	maxCallChainExportLimit     = 2000
	callChainSummaryEventLimit  = 3
	callChainSummaryTextLimit   = 500
	callChainSummaryTimelineMax = 5
)

var requestLogSectionHeaderRE = regexp.MustCompile(`^===\s+(.+?)\s+===$`)

type callChainExportPayload struct {
	Version           int                      `json:"version"`
	ExportedAt        time.Time                `json:"exported_at"`
	LogDirectory      string                   `json:"log_directory"`
	RequestLogEnabled bool                     `json:"request_log_enabled"`
	Filters           callChainExportFilters   `json:"filters"`
	SessionCount      int                      `json:"session_count"`
	RequestCount      int                      `json:"request_count"`
	MatchedFileCount  int                      `json:"matched_file_count"`
	Sessions          []callChainSessionExport `json:"sessions"`
	Warnings          []string                 `json:"warnings,omitempty"`
}

type callChainExportFilters struct {
	SessionID     string `json:"session_id,omitempty"`
	RequestID     string `json:"request_id,omitempty"`
	Query         string `json:"query,omitempty"`
	From          string `json:"from,omitempty"`
	To            string `json:"to,omitempty"`
	Limit         int    `json:"limit"`
	IncludeErrors bool   `json:"include_errors"`
	IncludeRaw    bool   `json:"include_raw"`
	Summary       bool   `json:"summary,omitempty"`
}

type callChainSessionExport struct {
	ID          string                   `json:"id"`
	StartedAt   string                   `json:"started_at,omitempty"`
	EndedAt     string                   `json:"ended_at,omitempty"`
	Identifiers map[string][]string      `json:"identifiers,omitempty"`
	Requests    []callChainRequestExport `json:"requests"`
}

type callChainRequestExport struct {
	RequestID    string                 `json:"request_id,omitempty"`
	File         string                 `json:"file"`
	Size         int64                  `json:"size"`
	ModifiedAt   string                 `json:"modified_at,omitempty"`
	Timestamp    string                 `json:"timestamp,omitempty"`
	URL          string                 `json:"url,omitempty"`
	Method       string                 `json:"method,omitempty"`
	Status       int                    `json:"status,omitempty"`
	Transport    callChainTransport     `json:"transport,omitempty"`
	Model        string                 `json:"model,omitempty"`
	Identifiers  map[string][]string    `json:"identifiers,omitempty"`
	UserInputs   []callChainEvent       `json:"user_inputs,omitempty"`
	ModelOutputs []callChainEvent       `json:"model_outputs,omitempty"`
	Reasoning    []callChainEvent       `json:"reasoning,omitempty"`
	ToolCalls    []callChainEvent       `json:"tool_calls,omitempty"`
	ToolResults  []callChainEvent       `json:"tool_results,omitempty"`
	HTTP         callChainHTTPTrace     `json:"http"`
	RawSections  []callChainRawSection  `json:"raw_sections,omitempty"`
	Summary      *callChainRequestStats `json:"summary,omitempty"`

	timestampValue time.Time
}

type callChainRequestStats struct {
	UserInputCount          int `json:"user_input_count"`
	ModelOutputCount        int `json:"model_output_count"`
	ReasoningCount          int `json:"reasoning_count"`
	ToolCallCount           int `json:"tool_call_count"`
	ToolResultCount         int `json:"tool_result_count"`
	UpstreamRequestCount    int `json:"upstream_request_count"`
	UpstreamResponseCount   int `json:"upstream_response_count"`
	WebsocketEventCount     int `json:"websocket_event_count"`
	APIWebsocketEventCount  int `json:"api_websocket_event_count"`
	RawSectionCount         int `json:"raw_section_count"`
	DownstreamRequestBytes  int `json:"downstream_request_bytes"`
	DownstreamResponseBytes int `json:"downstream_response_bytes"`
	UpstreamRequestBytes    int `json:"upstream_request_bytes"`
	UpstreamResponseBytes   int `json:"upstream_response_bytes"`
}

type callChainTransport struct {
	Downstream string `json:"downstream,omitempty"`
	Upstream   string `json:"upstream,omitempty"`
}

type callChainHTTPTrace struct {
	DownstreamRequest    callChainHTTPRequest       `json:"downstream_request"`
	UpstreamRequests     []callChainUpstreamRequest `json:"upstream_requests,omitempty"`
	UpstreamResponses    []callChainHTTPResponse    `json:"upstream_responses,omitempty"`
	DownstreamResponse   callChainHTTPResponse      `json:"downstream_response,omitempty"`
	WebsocketTimeline    []callChainTimelineEvent   `json:"websocket_timeline,omitempty"`
	APIWebsocketTimeline []callChainTimelineEvent   `json:"api_websocket_timeline,omitempty"`
}

type callChainHTTPRequest struct {
	URL     string              `json:"url,omitempty"`
	Method  string              `json:"method,omitempty"`
	Headers map[string][]string `json:"headers,omitempty"`
	Body    string              `json:"body,omitempty"`
}

type callChainUpstreamRequest struct {
	Index     int                 `json:"index,omitempty"`
	Timestamp string              `json:"timestamp,omitempty"`
	URL       string              `json:"url,omitempty"`
	Method    string              `json:"method,omitempty"`
	Auth      string              `json:"auth,omitempty"`
	Headers   map[string][]string `json:"headers,omitempty"`
	Body      string              `json:"body,omitempty"`
}

type callChainHTTPResponse struct {
	Index     int                 `json:"index,omitempty"`
	Timestamp string              `json:"timestamp,omitempty"`
	Status    int                 `json:"status,omitempty"`
	Headers   map[string][]string `json:"headers,omitempty"`
	Body      string              `json:"body,omitempty"`
}

type callChainTimelineEvent struct {
	Timestamp string `json:"timestamp,omitempty"`
	Event     string `json:"event,omitempty"`
	Payload   string `json:"payload,omitempty"`
}

type callChainEvent struct {
	Source string `json:"source,omitempty"`
	Path   string `json:"path,omitempty"`
	Type   string `json:"type,omitempty"`
	Role   string `json:"role,omitempty"`
	Name   string `json:"name,omitempty"`
	CallID string `json:"call_id,omitempty"`
	Text   string `json:"text,omitempty"`
	Raw    string `json:"raw,omitempty"`
}

type callChainRawSection struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

type requestLogSection struct {
	Title   string
	Content string
}

type callChainExportOptions struct {
	filters callChainExportFilters
	from    time.Time
	to      time.Time
}

type requestLogCandidate struct {
	path    string
	name    string
	info    os.FileInfo
	modTime time.Time
}

// ExportRequestCallChain exports structured request logs grouped into conversation/call chains.
func (h *Handler) ExportRequestCallChain(c *gin.Context) {
	if h == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "handler unavailable"})
		return
	}
	if h.cfg == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "configuration unavailable"})
		return
	}

	dir := h.logDirectory()
	if strings.TrimSpace(dir) == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "log directory not configured"})
		return
	}

	opts, errOpts := parseCallChainExportOptions(c)
	if errOpts != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": errOpts.Error()})
		return
	}

	payload, errExport := h.buildRequestCallChainExport(dir, opts)
	if errExport != nil {
		if os.IsNotExist(errExport) {
			c.JSON(http.StatusOK, callChainExportPayload{
				Version:           callChainExportVersion,
				ExportedAt:        time.Now().UTC(),
				LogDirectory:      dir,
				RequestLogEnabled: h.cfg.RequestLog,
				Filters:           opts.filters,
				Sessions:          []callChainSessionExport{},
				Warnings:          []string{"log directory not found"},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to export request call chain: %v", errExport)})
		return
	}

	filename := fmt.Sprintf("request-call-chain-%s.json", time.Now().UTC().Format("20060102T150405Z"))
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Header("X-Content-Type-Options", "nosniff")
	encoder := json.NewEncoder(c.Writer)
	encoder.SetIndent("", "  ")
	if errEncode := encoder.Encode(payload); errEncode != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to encode export: %v", errEncode)})
		return
	}
}

func parseCallChainExportOptions(c *gin.Context) (callChainExportOptions, error) {
	filters := callChainExportFilters{
		SessionID:     strings.TrimSpace(firstNonEmptyQuery(c, "session_id", "session", "conversation_id")),
		RequestID:     strings.TrimSpace(firstNonEmptyQuery(c, "request_id", "id")),
		Query:         strings.TrimSpace(firstNonEmptyQuery(c, "q", "query")),
		Limit:         defaultCallChainExportLimit,
		IncludeErrors: true,
	}

	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit <= 0 {
			return callChainExportOptions{}, fmt.Errorf("limit must be a positive integer")
		}
		if limit > maxCallChainExportLimit {
			return callChainExportOptions{}, fmt.Errorf("limit must be <= %d", maxCallChainExportLimit)
		}
		filters.Limit = limit
	}
	if raw := strings.TrimSpace(c.Query("include_errors")); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return callChainExportOptions{}, fmt.Errorf("include_errors must be a boolean")
		}
		filters.IncludeErrors = parsed
	}
	if raw := strings.TrimSpace(c.Query("include_raw")); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return callChainExportOptions{}, fmt.Errorf("include_raw must be a boolean")
		}
		filters.IncludeRaw = parsed
	}
	if raw := strings.TrimSpace(c.Query("summary")); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return callChainExportOptions{}, fmt.Errorf("summary must be a boolean")
		}
		filters.Summary = parsed
	}

	var from time.Time
	if raw := strings.TrimSpace(firstNonEmptyQuery(c, "from", "since", "start")); raw != "" {
		parsed, err := parseCallChainTime(raw)
		if err != nil {
			return callChainExportOptions{}, fmt.Errorf("from must be RFC3339 or unix timestamp")
		}
		from = parsed
		filters.From = parsed.Format(time.RFC3339Nano)
	}
	var to time.Time
	if raw := strings.TrimSpace(firstNonEmptyQuery(c, "to", "until", "end")); raw != "" {
		parsed, err := parseCallChainTime(raw)
		if err != nil {
			return callChainExportOptions{}, fmt.Errorf("to must be RFC3339 or unix timestamp")
		}
		to = parsed
		filters.To = parsed.Format(time.RFC3339Nano)
	}
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		return callChainExportOptions{}, fmt.Errorf("from must be before to")
	}

	return callChainExportOptions{filters: filters, from: from, to: to}, nil
}

func firstNonEmptyQuery(c *gin.Context, keys ...string) string {
	if c == nil {
		return ""
	}
	for _, key := range keys {
		if value := strings.TrimSpace(c.Query(key)); value != "" {
			return value
		}
	}
	return ""
}

func parseCallChainTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("empty timestamp")
	}
	if unix, err := strconv.ParseInt(raw, 10, 64); err == nil && unix > 0 {
		return time.Unix(unix, 0).UTC(), nil
	}
	return time.Parse(time.RFC3339Nano, raw)
}

func (h *Handler) buildRequestCallChainExport(dir string, opts callChainExportOptions) (callChainExportPayload, error) {
	candidates, err := collectRequestLogCandidates(dir, opts.filters.IncludeErrors)
	if err != nil {
		return callChainExportPayload{}, err
	}

	payload := callChainExportPayload{
		Version:           callChainExportVersion,
		ExportedAt:        time.Now().UTC(),
		LogDirectory:      dir,
		RequestLogEnabled: h.cfg != nil && h.cfg.RequestLog,
		Filters:           opts.filters,
		Sessions:          []callChainSessionExport{},
	}

	requests := make([]callChainRequestExport, 0, minInt(len(candidates), opts.filters.Limit))
	warnings := make([]string, 0)
	for _, candidate := range candidates {
		if opts.filters.Limit > 0 && len(requests) >= opts.filters.Limit {
			break
		}
		raw, errRead := os.ReadFile(candidate.path)
		if errRead != nil {
			warnings = append(warnings, fmt.Sprintf("failed to read %s: %v", candidate.name, errRead))
			continue
		}
		if !matchesCallChainRawFilters(candidate, raw, opts) {
			continue
		}
		req, errParse := parseRequestLogFile(candidate, raw, opts.filters.IncludeRaw)
		if errParse != nil {
			warnings = append(warnings, fmt.Sprintf("failed to parse %s: %v", candidate.name, errParse))
			continue
		}
		if !matchesCallChainParsedFilters(req, string(raw), opts) {
			continue
		}
		requests = append(requests, req)
	}

	sessions := groupCallChainRequests(requests, opts.filters.SessionID)
	if opts.filters.Summary {
		sessions = summarizeCallChainSessions(sessions)
	}
	payload.Sessions = sessions
	payload.SessionCount = len(sessions)
	payload.RequestCount = len(requests)
	payload.MatchedFileCount = len(requests)
	if len(warnings) > 0 {
		payload.Warnings = warnings
	}
	return payload, nil
}

func collectRequestLogCandidates(dir string, includeErrors bool) ([]requestLogCandidate, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	candidates := make([]requestLogCandidate, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == defaultLogFileName || isRotatedLogFile(name) || !strings.HasSuffix(name, ".log") {
			continue
		}
		if strings.HasPrefix(name, "error-") && !includeErrors {
			continue
		}
		info, errInfo := entry.Info()
		if errInfo != nil {
			continue
		}
		candidates = append(candidates, requestLogCandidate{
			path:    filepath.Join(dir, name),
			name:    name,
			info:    info,
			modTime: info.ModTime(),
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].modTime.After(candidates[j].modTime)
	})
	return candidates, nil
}

func matchesCallChainRawFilters(candidate requestLogCandidate, raw []byte, opts callChainExportOptions) bool {
	if opts.filters.RequestID != "" && !strings.Contains(candidate.name, "-"+opts.filters.RequestID+".log") {
		return false
	}
	if !opts.from.IsZero() && candidate.modTime.Before(opts.from) {
		return false
	}
	if !opts.to.IsZero() && candidate.modTime.After(opts.to) {
		return false
	}
	return true
}

func matchesCallChainParsedFilters(req callChainRequestExport, raw string, opts callChainExportOptions) bool {
	ts := req.timestampValue
	if ts.IsZero() {
		if parsed, err := time.Parse(time.RFC3339Nano, req.Timestamp); err == nil {
			ts = parsed
		}
	}
	if !opts.from.IsZero() && !ts.IsZero() && ts.Before(opts.from) {
		return false
	}
	if !opts.to.IsZero() && !ts.IsZero() && ts.After(opts.to) {
		return false
	}
	if opts.filters.Query != "" && !requestMatchesQuery(req, raw, opts.filters.Query) {
		return false
	}
	if opts.filters.SessionID != "" && !requestMatchesSessionID(req, raw, opts.filters.SessionID) {
		return false
	}
	return true
}

func requestMatchesQuery(req callChainRequestExport, raw string, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return true
	}
	if strings.Contains(strings.ToLower(raw), query) {
		return true
	}
	for _, value := range callChainSearchValues(req) {
		if strings.Contains(strings.ToLower(value), query) {
			return true
		}
	}
	return false
}

func callChainSearchValues(req callChainRequestExport) []string {
	values := []string{
		req.RequestID,
		req.File,
		req.Timestamp,
		req.URL,
		req.Method,
		req.Model,
	}
	for key, list := range req.Identifiers {
		values = append(values, key)
		values = append(values, list...)
	}
	appendEvents := func(events []callChainEvent) {
		for _, event := range events {
			values = append(values, event.Source, event.Path, event.Type, event.Role, event.Name, event.CallID, event.Text, event.Raw)
		}
	}
	appendEvents(req.UserInputs)
	appendEvents(req.ModelOutputs)
	appendEvents(req.Reasoning)
	appendEvents(req.ToolCalls)
	appendEvents(req.ToolResults)

	values = appendPayloadSearchText(values, req.HTTP.DownstreamRequest.Body)
	values = appendPayloadSearchText(values, req.HTTP.DownstreamResponse.Body)
	for _, upstream := range req.HTTP.UpstreamRequests {
		values = append(values, upstream.URL, upstream.Method, upstream.Auth)
		values = appendPayloadSearchText(values, upstream.Body)
	}
	for _, upstream := range req.HTTP.UpstreamResponses {
		values = appendPayloadSearchText(values, upstream.Body)
	}
	for _, event := range req.HTTP.WebsocketTimeline {
		values = append(values, event.Event)
		values = appendPayloadSearchText(values, event.Payload)
	}
	for _, event := range req.HTTP.APIWebsocketTimeline {
		values = append(values, event.Event)
		values = appendPayloadSearchText(values, event.Payload)
	}
	for _, section := range req.RawSections {
		values = append(values, section.Title, section.Content)
	}
	return values
}

func appendPayloadSearchText(values []string, payload string) []string {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return values
	}
	values = append(values, payload)
	for _, candidate := range extractJSONPayloadsFromText(payload) {
		var decoded any
		decoder := json.NewDecoder(strings.NewReader(candidate))
		decoder.UseNumber()
		if err := decoder.Decode(&decoded); err != nil {
			continue
		}
		if normalized := marshalCompact(decoded); normalized != "" {
			values = append(values, normalized)
		}
	}
	return values
}

func requestMatchesSessionID(req callChainRequestExport, raw string, sessionID string) bool {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return true
	}
	if strings.Contains(raw, sessionID) {
		return true
	}
	if req.RequestID != "" && (sessionID == req.RequestID || sessionID == "request:"+req.RequestID) {
		return true
	}
	if req.File != "" && (sessionID == req.File || sessionID == "file:"+req.File) {
		return true
	}
	if responseID := strings.TrimPrefix(sessionID, "response-chain:"); responseID != sessionID && responseID != "" {
		return identifierHasValue(req.Identifiers, "response_id", responseID) ||
			identifierHasValue(req.Identifiers, "previous_response_id", responseID)
	}
	if responseID := strings.TrimPrefix(sessionID, "response:"); responseID != sessionID && responseID != "" {
		return identifierHasValue(req.Identifiers, "response_id", responseID) ||
			identifierHasValue(req.Identifiers, "previous_response_id", responseID)
	}
	for _, values := range req.Identifiers {
		for _, value := range values {
			if value == sessionID {
				return true
			}
		}
	}
	return false
}

func identifierHasValue(identifiers map[string][]string, key string, value string) bool {
	for _, existing := range identifiers[key] {
		if existing == value {
			return true
		}
	}
	return false
}

func parseRequestLogFile(candidate requestLogCandidate, raw []byte, includeRaw bool) (callChainRequestExport, error) {
	sections := splitRequestLogSections(string(raw))
	req := callChainRequestExport{
		RequestID:   requestIDFromLogFilename(candidate.name),
		File:        candidate.name,
		Size:        candidate.info.Size(),
		ModifiedAt:  candidate.info.ModTime().UTC().Format(time.RFC3339Nano),
		Identifiers: make(map[string][]string),
		HTTP: callChainHTTPTrace{
			UpstreamRequests:     []callChainUpstreamRequest{},
			UpstreamResponses:    []callChainHTTPResponse{},
			WebsocketTimeline:    []callChainTimelineEvent{},
			APIWebsocketTimeline: []callChainTimelineEvent{},
		},
	}
	extracted := newCallChainExtraction()

	for _, section := range sections {
		title := strings.ToUpper(strings.TrimSpace(section.Title))
		switch {
		case title == "REQUEST INFO":
			fields := parseCallChainFields(section.Content)
			req.URL = fields["URL"]
			req.Method = fields["Method"]
			req.Timestamp = normalizeCallChainTimestamp(fields["Timestamp"])
			if ts, err := time.Parse(time.RFC3339Nano, req.Timestamp); err == nil {
				req.timestampValue = ts
			}
			req.Transport.Downstream = fields["Downstream Transport"]
			req.Transport.Upstream = fields["Upstream Transport"]
			req.HTTP.DownstreamRequest.URL = req.URL
			req.HTTP.DownstreamRequest.Method = req.Method
		case title == "HEADERS":
			req.HTTP.DownstreamRequest.Headers = parseCallChainHeaders(section.Content)
			addHeaderIdentifiers(req.HTTP.DownstreamRequest.Headers, extracted)
		case title == "REQUEST BODY":
			req.HTTP.DownstreamRequest.Body = strings.TrimSpace(section.Content)
			inspectCallChainJSONPayload(req.HTTP.DownstreamRequest.Body, "downstream.request.body", extracted)
		case strings.HasPrefix(title, "API REQUEST"):
			upReq := parseCallChainAPIRequest(section)
			req.HTTP.UpstreamRequests = append(req.HTTP.UpstreamRequests, upReq)
			inspectCallChainJSONPayload(upReq.Body, fmt.Sprintf("upstream.request.%d.body", upReq.Index), extracted)
		case strings.HasPrefix(title, "API RESPONSE"):
			upResp := parseCallChainAPIResponse(section)
			req.HTTP.UpstreamResponses = append(req.HTTP.UpstreamResponses, upResp)
			inspectCallChainJSONPayloadsFromBody(upResp.Body, fmt.Sprintf("upstream.response.%d.body", upResp.Index), extracted)
		case title == "RESPONSE":
			downResp := parseCallChainResponse(section.Content)
			req.HTTP.DownstreamResponse = downResp
			req.Status = downResp.Status
			inspectCallChainJSONPayloadsFromBody(downResp.Body, "downstream.response.body", extracted)
		case title == "WEBSOCKET TIMELINE":
			req.HTTP.WebsocketTimeline = parseCallChainTimeline(section.Content)
			for i, event := range req.HTTP.WebsocketTimeline {
				inspectCallChainJSONPayload(event.Payload, fmt.Sprintf("websocket.%d.%s", i+1, event.Event), extracted)
			}
		case title == "API WEBSOCKET TIMELINE":
			req.HTTP.APIWebsocketTimeline = parseCallChainTimeline(section.Content)
			for i, event := range req.HTTP.APIWebsocketTimeline {
				inspectCallChainJSONPayload(event.Payload, fmt.Sprintf("api_websocket.%d.%s", i+1, event.Event), extracted)
			}
		}
		if includeRaw {
			req.RawSections = append(req.RawSections, callChainRawSection{Title: section.Title, Content: section.Content})
		}
	}

	req.Identifiers = extracted.identifiers
	req.UserInputs = extracted.userInputs
	req.ModelOutputs = extracted.modelOutputs
	req.Reasoning = extracted.reasoning
	req.ToolCalls = extracted.toolCalls
	req.ToolResults = extracted.toolResults
	req.Model = firstIdentifierValue(req.Identifiers, "model")
	if req.Model == "" {
		req.Model = firstModelFromBodies(req)
	}
	if len(req.Identifiers) == 0 {
		req.Identifiers = nil
	}
	if len(req.HTTP.UpstreamRequests) == 0 {
		req.HTTP.UpstreamRequests = nil
	}
	if len(req.HTTP.UpstreamResponses) == 0 {
		req.HTTP.UpstreamResponses = nil
	}
	if len(req.HTTP.WebsocketTimeline) == 0 {
		req.HTTP.WebsocketTimeline = nil
	}
	if len(req.HTTP.APIWebsocketTimeline) == 0 {
		req.HTTP.APIWebsocketTimeline = nil
	}
	return req, nil
}

func splitRequestLogSections(raw string) []requestLogSection {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	sections := make([]requestLogSection, 0, 8)
	var currentTitle string
	var content strings.Builder
	flush := func() {
		if currentTitle == "" {
			return
		}
		sections = append(sections, requestLogSection{
			Title:   currentTitle,
			Content: strings.TrimRight(content.String(), "\n"),
		})
		content.Reset()
	}
	for _, line := range lines {
		if match := requestLogSectionHeaderRE.FindStringSubmatch(line); len(match) == 2 {
			flush()
			currentTitle = strings.TrimSpace(match[1])
			continue
		}
		if currentTitle == "" {
			continue
		}
		content.WriteString(line)
		content.WriteString("\n")
	}
	flush()
	return sections
}

func parseCallChainFields(content string) map[string]string {
	fields := make(map[string]string)
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.EqualFold(line, "Headers:") || strings.EqualFold(line, "Body:") {
			break
		}
		idx := strings.Index(line, ":")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		fields[key] = value
	}
	return fields
}

func parseCallChainHeaders(content string) map[string][]string {
	headers := make(map[string][]string)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "<none>" {
			continue
		}
		idx := strings.Index(line, ":")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		headers[key] = append(headers[key], value)
	}
	if len(headers) == 0 {
		return nil
	}
	return headers
}

func parseCallChainAPIRequest(section requestLogSection) callChainUpstreamRequest {
	meta, headers, body := splitCallChainHTTPPayload(section.Content)
	fields := parseCallChainFields(meta)
	return callChainUpstreamRequest{
		Index:     parseCallChainSectionIndex(section.Title),
		Timestamp: normalizeCallChainTimestamp(fields["Timestamp"]),
		URL:       fields["Upstream URL"],
		Method:    fields["HTTP Method"],
		Auth:      fields["Auth"],
		Headers:   parseCallChainHeaders(headers),
		Body:      strings.TrimSpace(body),
	}
}

func parseCallChainAPIResponse(section requestLogSection) callChainHTTPResponse {
	meta, headers, body := splitCallChainHTTPPayload(section.Content)
	fields := parseCallChainFields(meta)
	status, _ := strconv.Atoi(strings.TrimSpace(fields["Status"]))
	return callChainHTTPResponse{
		Index:     parseCallChainSectionIndex(section.Title),
		Timestamp: normalizeCallChainTimestamp(fields["Timestamp"]),
		Status:    status,
		Headers:   parseCallChainHeaders(headers),
		Body:      strings.TrimSpace(body),
	}
}

func parseCallChainResponse(content string) callChainHTTPResponse {
	parts := strings.SplitN(content, "\n\n", 2)
	head := content
	body := ""
	if len(parts) == 2 {
		head = parts[0]
		body = parts[1]
	}
	fields := parseCallChainFields(head)
	status, _ := strconv.Atoi(strings.TrimSpace(fields["Status"]))
	return callChainHTTPResponse{
		Status:  status,
		Headers: parseCallChainHeaders(removeStatusLine(head)),
		Body:    strings.TrimSpace(body),
	}
}

func splitCallChainHTTPPayload(content string) (meta string, headers string, body string) {
	beforeBody := content
	if parts := strings.SplitN(content, "\nBody:\n", 2); len(parts) == 2 {
		beforeBody = parts[0]
		body = parts[1]
	}
	if parts := strings.SplitN(beforeBody, "\nHeaders:\n", 2); len(parts) == 2 {
		meta = parts[0]
		headers = parts[1]
		return meta, headers, body
	}
	return beforeBody, "", body
}

func removeStatusLine(content string) string {
	lines := strings.Split(content, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "Status:") {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.Join(filtered, "\n")
}

func parseCallChainSectionIndex(title string) int {
	fields := strings.Fields(title)
	if len(fields) == 0 {
		return 0
	}
	n, _ := strconv.Atoi(fields[len(fields)-1])
	return n
}

func normalizeCallChainTimestamp(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return parsed.UTC().Format(time.RFC3339Nano)
	}
	return raw
}

func parseCallChainTimeline(content string) []callChainTimelineEvent {
	lines := strings.Split(content, "\n")
	events := make([]callChainTimelineEvent, 0)
	var current *callChainTimelineEvent
	var payload strings.Builder
	flush := func() {
		if current == nil {
			return
		}
		current.Payload = strings.TrimSpace(payload.String())
		events = append(events, *current)
		payload.Reset()
		current = nil
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Timestamp:") {
			flush()
			current = &callChainTimelineEvent{Timestamp: normalizeCallChainTimestamp(strings.TrimSpace(strings.TrimPrefix(trimmed, "Timestamp:")))}
			continue
		}
		if current != nil && strings.HasPrefix(trimmed, "Event:") {
			current.Event = strings.TrimSpace(strings.TrimPrefix(trimmed, "Event:"))
			continue
		}
		if current == nil {
			continue
		}
		payload.WriteString(line)
		payload.WriteString("\n")
	}
	flush()
	return events
}

type callChainExtraction struct {
	identifiers  map[string][]string
	seenEvents   map[string]struct{}
	userInputs   []callChainEvent
	modelOutputs []callChainEvent
	reasoning    []callChainEvent
	toolCalls    []callChainEvent
	toolResults  []callChainEvent
}

func newCallChainExtraction() *callChainExtraction {
	return &callChainExtraction{
		identifiers: make(map[string][]string),
		seenEvents:  make(map[string]struct{}),
	}
}

func addHeaderIdentifiers(headers map[string][]string, out *callChainExtraction) {
	for key, values := range headers {
		normalized := strings.ToLower(strings.TrimSpace(key))
		switch normalized {
		case "x-session-id":
			for _, value := range values {
				addIdentifier(out.identifiers, "x_session_id", value)
			}
		case "idempotency-key":
			for _, value := range values {
				addIdentifier(out.identifiers, "idempotency_key", value)
			}
		case "x-codex-turn-state":
			for _, value := range values {
				addIdentifier(out.identifiers, "x_codex_turn_state", value)
			}
		}
	}
}

func inspectCallChainJSONPayloadsFromBody(body string, source string, out *callChainExtraction) {
	body = strings.TrimSpace(body)
	if body == "" {
		return
	}
	inspected := false
	for _, payload := range extractJSONPayloadsFromText(body) {
		inspectCallChainJSONPayload(payload, source, out)
		inspected = true
	}
	if !inspected {
		inspectCallChainJSONPayload(body, source, out)
	}
}

func inspectCallChainJSONPayload(raw string, source string, out *callChainExtraction) {
	raw = strings.TrimSpace(raw)
	if raw == "" || out == nil {
		return
	}
	if strings.HasPrefix(raw, "data:") {
		raw = strings.TrimSpace(strings.TrimPrefix(raw, "data:"))
	}
	if raw == "[DONE]" || !json.Valid([]byte(raw)) {
		return
	}
	var decoded any
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return
	}
	walkCallChainJSON(decoded, nil, source, out)
}

func extractJSONPayloadsFromText(text string) []string {
	lines := strings.Split(text, "\n")
	payloads := make([]string, 0)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "event:") {
			continue
		}
		if strings.HasPrefix(trimmed, "data:") {
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
		}
		if trimmed == "" || trimmed == "[DONE]" {
			continue
		}
		if json.Valid([]byte(trimmed)) {
			payloads = append(payloads, trimmed)
		}
	}
	if len(payloads) > 0 {
		return payloads
	}
	if json.Valid([]byte(strings.TrimSpace(text))) {
		return []string{strings.TrimSpace(text)}
	}
	return nil
}

func walkCallChainJSON(value any, path []string, source string, out *callChainExtraction) {
	switch typed := value.(type) {
	case map[string]any:
		processCallChainObject(typed, path, source, out)
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			walkCallChainJSON(typed[key], append(path, key), source, out)
		}
	case []any:
		for i, item := range typed {
			walkCallChainJSON(item, append(path, strconv.Itoa(i)), source, out)
		}
	}
}

func processCallChainObject(obj map[string]any, path []string, source string, out *callChainExtraction) {
	addObjectIdentifiers(obj, out)

	typ := strings.TrimSpace(callChainStringValue(obj["type"]))
	role := strings.TrimSpace(callChainStringValue(obj["role"]))
	pathText := strings.Join(path, ".")

	if typ == "message" || role == "user" || role == "assistant" || role == "tool" {
		texts := textValuesFromContent(obj["content"])
		if len(texts) == 0 {
			texts = textValuesFromContent(obj["text"])
		}
		for _, text := range texts {
			event := callChainEvent{Source: source, Path: pathText, Type: typ, Role: role, Text: text}
			switch role {
			case "user":
				addCallChainEvent(&out.userInputs, out, "user", event)
			case "assistant":
				addCallChainEvent(&out.modelOutputs, out, "assistant", event)
			case "tool":
				event.CallID = firstStringValue(obj, "tool_call_id", "call_id", "id")
				addCallChainEvent(&out.toolResults, out, "tool_result", event)
			}
		}
	}

	switch typ {
	case "response.output_text.delta", "content_block_delta":
		if text := firstStringValue(obj, "delta", "text"); text != "" {
			addCallChainEvent(&out.modelOutputs, out, "output_delta", callChainEvent{Source: source, Path: pathText, Type: typ, Text: text})
		}
	case "response.reasoning_summary_text.delta", "reasoning_delta", "thinking_delta":
		if text := firstStringValue(obj, "delta", "text"); text != "" {
			addCallChainEvent(&out.reasoning, out, "reasoning_delta", callChainEvent{Source: source, Path: pathText, Type: typ, Text: text})
		}
	case "function_call", "custom_tool_call":
		addToolCallEvent(obj, pathText, source, typ, out)
	case "function_call_output", "custom_tool_call_output":
		addToolResultEvent(obj, pathText, source, typ, out)
	case "tool_use":
		addToolCallEvent(obj, pathText, source, typ, out)
	case "tool_result":
		addToolResultEvent(obj, pathText, source, typ, out)
	}

	if toolCalls, ok := obj["tool_calls"].([]any); ok {
		for i, item := range toolCalls {
			if toolCall, ok := item.(map[string]any); ok {
				addToolCallEvent(toolCall, pathText+".tool_calls."+strconv.Itoa(i), source, "tool_call", out)
			}
		}
	}
	if functionCall, ok := obj["functionCall"].(map[string]any); ok {
		addToolCallEvent(functionCall, pathText+".functionCall", source, "function_call", out)
	}
	if functionResponse, ok := obj["functionResponse"].(map[string]any); ok {
		addToolResultEvent(functionResponse, pathText+".functionResponse", source, "function_response", out)
	}

	for key, value := range obj {
		lowerKey := strings.ToLower(key)
		if strings.Contains(lowerKey, "reasoning") || strings.Contains(lowerKey, "thinking") {
			raw := marshalCompact(value)
			text := callChainStringValue(value)
			if text == "" {
				text = raw
			}
			if text != "" {
				addCallChainEvent(&out.reasoning, out, "reasoning", callChainEvent{Source: source, Path: joinJSONPath(path, key), Type: key, Text: text, Raw: raw})
			}
		}
	}
}

func addObjectIdentifiers(obj map[string]any, out *callChainExtraction) {
	for key, value := range obj {
		normalized := strings.ToLower(strings.TrimSpace(key))
		switch normalized {
		case "model":
			addIdentifier(out.identifiers, "model", callChainStringValue(value))
		case "conversation_id", "conversationid":
			addIdentifier(out.identifiers, "conversation_id", callChainStringValue(value))
		case "session_id", "sessionid":
			addIdentifier(out.identifiers, "session_id", callChainStringValue(value))
		case "previous_response_id":
			addIdentifier(out.identifiers, "previous_response_id", callChainStringValue(value))
		case "id":
			if id := callChainStringValue(value); strings.HasPrefix(id, "resp_") || strings.HasPrefix(id, "response_") {
				addIdentifier(out.identifiers, "response_id", id)
			}
		case "call_id", "tool_call_id":
			addIdentifier(out.identifiers, normalized, callChainStringValue(value))
		case "user_id", "userid":
			addIdentifier(out.identifiers, "user_id", callChainStringValue(value))
		}
	}
	if metadata, ok := obj["metadata"].(map[string]any); ok {
		addIdentifier(out.identifiers, "metadata_user_id", callChainStringValue(metadata["user_id"]))
	}
	if response, ok := obj["response"].(map[string]any); ok {
		addIdentifier(out.identifiers, "response_id", callChainStringValue(response["id"]))
	}
}

func addToolCallEvent(obj map[string]any, path string, source string, typ string, out *callChainExtraction) {
	event := callChainEvent{
		Source: source,
		Path:   path,
		Type:   typ,
		Name:   firstStringValue(obj, "name"),
		CallID: firstStringValue(obj, "call_id", "id", "tool_call_id"),
	}
	if functionObj, ok := obj["function"].(map[string]any); ok {
		if event.Name == "" {
			event.Name = callChainStringValue(functionObj["name"])
		}
		if args := functionObj["arguments"]; args != nil {
			event.Raw = stringOrCompact(args)
		}
	}
	if event.Raw == "" {
		event.Raw = firstStringValue(obj, "arguments", "input", "args")
	}
	if event.Raw == "" {
		event.Raw = marshalCompact(obj)
	}
	addCallChainEvent(&out.toolCalls, out, "tool_call", event)
}

func addToolResultEvent(obj map[string]any, path string, source string, typ string, out *callChainExtraction) {
	event := callChainEvent{
		Source: source,
		Path:   path,
		Type:   typ,
		Name:   firstStringValue(obj, "name"),
		CallID: firstStringValue(obj, "call_id", "id", "tool_call_id"),
	}
	if output := firstStringValue(obj, "output", "content", "result"); output != "" {
		event.Text = output
	} else {
		event.Raw = marshalCompact(obj)
	}
	addCallChainEvent(&out.toolResults, out, "tool_result", event)
}

func textValuesFromContent(value any) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return []string{typed}
	case map[string]any:
		for _, key := range []string{"text", "input_text", "output_text"} {
			if text := callChainStringValue(typed[key]); text != "" {
				return []string{text}
			}
		}
	case []any:
		values := make([]string, 0)
		for _, item := range typed {
			values = append(values, textValuesFromContent(item)...)
		}
		return values
	}
	return nil
}

func addCallChainEvent(target *[]callChainEvent, extraction *callChainExtraction, prefix string, event callChainEvent) {
	event.Text = strings.TrimSpace(event.Text)
	event.Raw = strings.TrimSpace(event.Raw)
	if event.Text == "" && event.Raw == "" && event.Name == "" && event.CallID == "" {
		return
	}
	key := prefix + "|" + event.Source + "|" + event.Path + "|" + event.Type + "|" + event.Role + "|" + event.Name + "|" + event.CallID + "|" + event.Text + "|" + event.Raw
	if _, ok := extraction.seenEvents[key]; ok {
		return
	}
	extraction.seenEvents[key] = struct{}{}
	*target = append(*target, event)
}

func addIdentifier(target map[string][]string, key string, value string) {
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" || value == "" {
		return
	}
	for _, existing := range target[key] {
		if existing == value {
			return
		}
	}
	target[key] = append(target[key], value)
}

func callChainStringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(typed)
	default:
		return ""
	}
}

func firstStringValue(obj map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := callChainStringValue(obj[key]); strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func stringOrCompact(value any) string {
	if text := callChainStringValue(value); text != "" {
		return text
	}
	return marshalCompact(value)
}

func marshalCompact(value any) string {
	if value == nil {
		return ""
	}
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(data)
}

func joinJSONPath(path []string, key string) string {
	if len(path) == 0 {
		return key
	}
	joined := append([]string{}, path...)
	joined = append(joined, key)
	return strings.Join(joined, ".")
}

func firstIdentifierValue(identifiers map[string][]string, key string) string {
	values := identifiers[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func firstModelFromBodies(req callChainRequestExport) string {
	for _, body := range []string{req.HTTP.DownstreamRequest.Body, req.HTTP.DownstreamResponse.Body} {
		model := modelFromJSONText(body)
		if model != "" {
			return model
		}
	}
	for _, upstream := range req.HTTP.UpstreamRequests {
		if model := modelFromJSONText(upstream.Body); model != "" {
			return model
		}
	}
	for _, upstream := range req.HTTP.UpstreamResponses {
		if model := modelFromJSONText(upstream.Body); model != "" {
			return model
		}
	}
	return ""
}

func modelFromJSONText(raw string) string {
	for _, payload := range extractJSONPayloadsFromText(raw) {
		var decoded map[string]any
		decoder := json.NewDecoder(strings.NewReader(payload))
		decoder.UseNumber()
		if err := decoder.Decode(&decoded); err != nil {
			continue
		}
		if model := callChainStringValue(decoded["model"]); model != "" {
			return model
		}
		if response, ok := decoded["response"].(map[string]any); ok {
			if model := callChainStringValue(response["model"]); model != "" {
				return model
			}
		}
	}
	return ""
}

func groupCallChainRequests(requests []callChainRequestExport, forcedSessionID string) []callChainSessionExport {
	if len(requests) == 0 {
		return []callChainSessionExport{}
	}

	uf := newCallChainUnionFind()
	for i := range requests {
		nodeID := "request:" + fallbackRequestIdentity(requests[i], i)
		uf.add(nodeID)
		for _, key := range sessionIdentifierPriority() {
			for _, value := range requests[i].Identifiers[key] {
				uf.union(nodeID, key+":"+value)
			}
		}
		for _, key := range []string{"response_id", "previous_response_id"} {
			for _, value := range requests[i].Identifiers[key] {
				uf.union(nodeID, "response:"+value)
			}
		}
	}

	groups := make(map[string][]callChainRequestExport)
	for i := range requests {
		nodeID := "request:" + fallbackRequestIdentity(requests[i], i)
		root := uf.find(nodeID)
		groups[root] = append(groups[root], requests[i])
	}

	sessions := make([]callChainSessionExport, 0, len(groups))
	for _, groupedRequests := range groups {
		sort.Slice(groupedRequests, func(i, j int) bool {
			return requestTimeForSort(groupedRequests[i]).Before(requestTimeForSort(groupedRequests[j]))
		})
		session := callChainSessionExport{
			ID:          chooseCallChainSessionID(groupedRequests, forcedSessionID),
			Identifiers: mergeCallChainIdentifiers(groupedRequests),
			Requests:    groupedRequests,
		}
		if started := requestTimeForSort(groupedRequests[0]); !started.IsZero() {
			session.StartedAt = started.UTC().Format(time.RFC3339Nano)
		}
		if ended := requestTimeForSort(groupedRequests[len(groupedRequests)-1]); !ended.IsZero() {
			session.EndedAt = ended.UTC().Format(time.RFC3339Nano)
		}
		sessions = append(sessions, session)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].StartedAt < sessions[j].StartedAt
	})
	for i := range sessions {
		if len(sessions[i].Identifiers) == 0 {
			sessions[i].Identifiers = nil
		}
	}
	return sessions
}

func summarizeCallChainSessions(sessions []callChainSessionExport) []callChainSessionExport {
	for sessionIndex := range sessions {
		for requestIndex := range sessions[sessionIndex].Requests {
			summarizeCallChainRequest(&sessions[sessionIndex].Requests[requestIndex])
		}
	}
	return sessions
}

func summarizeCallChainRequest(req *callChainRequestExport) {
	if req == nil {
		return
	}
	req.Summary = buildCallChainRequestStats(*req)

	req.UserInputs = summarizeCallChainEvents(req.UserInputs)
	req.ModelOutputs = summarizeCallChainEvents(req.ModelOutputs)
	req.Reasoning = summarizeCallChainEvents(req.Reasoning)
	req.ToolCalls = summarizeCallChainEvents(req.ToolCalls)
	req.ToolResults = summarizeCallChainEvents(req.ToolResults)
	req.RawSections = nil

	req.HTTP.DownstreamRequest = callChainHTTPRequest{
		URL:    req.HTTP.DownstreamRequest.URL,
		Method: req.HTTP.DownstreamRequest.Method,
	}
	req.HTTP.DownstreamResponse = callChainHTTPResponse{
		Status: req.HTTP.DownstreamResponse.Status,
	}

	if len(req.HTTP.UpstreamRequests) > 0 {
		upstreamRequests := make([]callChainUpstreamRequest, 0, len(req.HTTP.UpstreamRequests))
		for _, upstream := range req.HTTP.UpstreamRequests {
			upstreamRequests = append(upstreamRequests, callChainUpstreamRequest{
				Index:     upstream.Index,
				Timestamp: upstream.Timestamp,
				URL:       upstream.URL,
				Method:    upstream.Method,
			})
		}
		req.HTTP.UpstreamRequests = upstreamRequests
	}
	if len(req.HTTP.UpstreamResponses) > 0 {
		upstreamResponses := make([]callChainHTTPResponse, 0, len(req.HTTP.UpstreamResponses))
		for _, upstream := range req.HTTP.UpstreamResponses {
			upstreamResponses = append(upstreamResponses, callChainHTTPResponse{
				Index:     upstream.Index,
				Timestamp: upstream.Timestamp,
				Status:    upstream.Status,
			})
		}
		req.HTTP.UpstreamResponses = upstreamResponses
	}
	req.HTTP.WebsocketTimeline = summarizeCallChainTimeline(req.HTTP.WebsocketTimeline)
	req.HTTP.APIWebsocketTimeline = summarizeCallChainTimeline(req.HTTP.APIWebsocketTimeline)
}

func buildCallChainRequestStats(req callChainRequestExport) *callChainRequestStats {
	stats := &callChainRequestStats{
		UserInputCount:          len(req.UserInputs),
		ModelOutputCount:        len(req.ModelOutputs),
		ReasoningCount:          len(req.Reasoning),
		ToolCallCount:           len(req.ToolCalls),
		ToolResultCount:         len(req.ToolResults),
		UpstreamRequestCount:    len(req.HTTP.UpstreamRequests),
		UpstreamResponseCount:   len(req.HTTP.UpstreamResponses),
		WebsocketEventCount:     len(req.HTTP.WebsocketTimeline),
		APIWebsocketEventCount:  len(req.HTTP.APIWebsocketTimeline),
		RawSectionCount:         len(req.RawSections),
		DownstreamRequestBytes:  len(req.HTTP.DownstreamRequest.Body),
		DownstreamResponseBytes: len(req.HTTP.DownstreamResponse.Body),
	}
	for _, upstream := range req.HTTP.UpstreamRequests {
		stats.UpstreamRequestBytes += len(upstream.Body)
	}
	for _, upstream := range req.HTTP.UpstreamResponses {
		stats.UpstreamResponseBytes += len(upstream.Body)
	}
	return stats
}

func summarizeCallChainEvents(events []callChainEvent) []callChainEvent {
	if len(events) == 0 {
		return nil
	}
	limit := minInt(len(events), callChainSummaryEventLimit)
	summary := make([]callChainEvent, 0, limit)
	for i := 0; i < limit; i++ {
		event := events[i]
		event.Text = truncateCallChainString(event.Text, callChainSummaryTextLimit)
		event.Raw = truncateCallChainString(event.Raw, callChainSummaryTextLimit)
		summary = append(summary, event)
	}
	return summary
}

func summarizeCallChainTimeline(events []callChainTimelineEvent) []callChainTimelineEvent {
	if len(events) == 0 {
		return nil
	}
	limit := minInt(len(events), callChainSummaryTimelineMax)
	summary := make([]callChainTimelineEvent, 0, limit)
	for i := 0; i < limit; i++ {
		event := events[i]
		event.Payload = truncateCallChainString(event.Payload, callChainSummaryTextLimit)
		summary = append(summary, event)
	}
	return summary
}

func truncateCallChainString(value string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes]) + "..."
}

func sessionIdentifierPriority() []string {
	return []string{
		"x_session_id",
		"conversation_id",
		"session_id",
		"metadata_user_id",
		"user_id",
		"x_codex_turn_state",
		"idempotency_key",
	}
}

func chooseCallChainSessionID(requests []callChainRequestExport, forced string) string {
	if strings.TrimSpace(forced) != "" {
		return strings.TrimSpace(forced)
	}
	for _, key := range sessionIdentifierPriority() {
		for _, req := range requests {
			if value := firstIdentifierValue(req.Identifiers, key); value != "" {
				return value
			}
		}
	}
	for _, req := range requests {
		if value := firstIdentifierValue(req.Identifiers, "response_id"); value != "" {
			return "response-chain:" + value
		}
		if value := firstIdentifierValue(req.Identifiers, "previous_response_id"); value != "" {
			return "response-chain:" + value
		}
	}
	if len(requests) > 0 && requests[0].RequestID != "" {
		return "request:" + requests[0].RequestID
	}
	return "session"
}

func mergeCallChainIdentifiers(requests []callChainRequestExport) map[string][]string {
	merged := make(map[string][]string)
	for _, req := range requests {
		for key, values := range req.Identifiers {
			for _, value := range values {
				addIdentifier(merged, key, value)
			}
		}
	}
	return merged
}

func fallbackRequestIdentity(req callChainRequestExport, index int) string {
	if req.RequestID != "" {
		return req.RequestID
	}
	if req.File != "" {
		return req.File
	}
	return strconv.Itoa(index)
}

func requestTimeForSort(req callChainRequestExport) time.Time {
	if !req.timestampValue.IsZero() {
		return req.timestampValue
	}
	if parsed, err := time.Parse(time.RFC3339Nano, req.Timestamp); err == nil {
		return parsed
	}
	if parsed, err := time.Parse(time.RFC3339Nano, req.ModifiedAt); err == nil {
		return parsed
	}
	return time.Time{}
}

func requestIDFromLogFilename(name string) string {
	name = strings.TrimSuffix(name, ".log")
	idx := strings.LastIndex(name, "-")
	if idx < 0 || idx == len(name)-1 {
		return ""
	}
	return name[idx+1:]
}

type callChainUnionFind struct {
	parent map[string]string
}

func newCallChainUnionFind() *callChainUnionFind {
	return &callChainUnionFind{parent: make(map[string]string)}
}

func (u *callChainUnionFind) add(value string) {
	if _, ok := u.parent[value]; !ok {
		u.parent[value] = value
	}
}

func (u *callChainUnionFind) find(value string) string {
	u.add(value)
	parent := u.parent[value]
	if parent == value {
		return value
	}
	root := u.find(parent)
	u.parent[value] = root
	return root
}

func (u *callChainUnionFind) union(left, right string) {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return
	}
	leftRoot := u.find(left)
	rightRoot := u.find(right)
	if leftRoot == rightRoot {
		return
	}
	if leftRoot < rightRoot {
		u.parent[rightRoot] = leftRoot
	} else {
		u.parent[leftRoot] = rightRoot
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
