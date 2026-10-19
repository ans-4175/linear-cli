package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

type object = map[string]any

func asObject(x any) (object, error) {
	m, ok := x.(map[string]any)
	if !ok || m == nil {
		return nil, fmt.Errorf("unexpected null/non-object API result")
	}
	return m, nil
}
func stringField(m object, k string) string { s, _ := m[k].(string); return s }
func connectionNodes(m object) ([]any, error) {
	nodes, ok := m["nodes"].([]any)
	if !ok {
		return nil, fmt.Errorf("API connection missing nodes")
	}
	return nodes, nil
}

var uuidRE = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var issueRE = regexp.MustCompile(`(?i)^([a-z][a-z0-9_]*)-([0-9]+)$`)
var dashlessRE = regexp.MustCompile(`(?i)^([a-z][a-z_]*)([0-9]+)$`)
var numberRE = regexp.MustCompile(`^[0-9]+$`)

func normalizeIssue(ref, team string) (string, error) {
	ref = strings.TrimSpace(ref)
	if uuidRE.MatchString(ref) {
		return strings.ToLower(ref), nil
	}
	if strings.Contains(ref, "://") {
		u, e := url.Parse(ref)
		if e != nil || u.Scheme != "https" || u.Hostname() != "linear.app" || u.Port() != "" {
			return "", usageError("issue URL must be an https://linear.app/.../issue/KEY-123/... URL")
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		found := ""
		for i, p := range parts {
			if p == "issue" && i+1 < len(parts) {
				found = parts[i+1]
				break
			}
		}
		if found == "" {
			return "", usageError("issue URL has no issue identifier")
		}
		ref = found
	}
	if m := issueRE.FindStringSubmatch(ref); m != nil {
		return strings.ToUpper(m[1]) + "-" + m[2], nil
	}
	if m := dashlessRE.FindStringSubmatch(ref); m != nil {
		return strings.ToUpper(m[1]) + "-" + m[2], nil
	}
	if numberRE.MatchString(ref) {
		if !aliasRE.MatchString(team) || uuidRE.MatchString(team) {
			return "", usageError("bare issue numbers require --team KEY; never defaults to the first team")
		}
		return strings.ToUpper(team) + "-" + ref, nil
	}
	return "", usageError("invalid issue reference; use KEY-123, UUID or Linear issue URL")
}

const issueSummary = "id identifier title priority url archivedAt state { id name type } assignee { id name email } team { id key name }"
const issueDetail = issueSummary + " description dueDate createdAt updatedAt parent { id identifier } project { id name } cycle { id name number } labels(first: 100) { nodes { id name } pageInfo { hasNextPage endCursor } }"
const commentFields = "id body url createdAt updatedAt user { id name }"
const pageFields = "pageInfo { hasNextPage endCursor }"

func (a *app) paginate(ctx context.Context, query string, vars object, root string) (object, error) {
	var merged []any
	seen := map[string]bool{}
	if cursor, ok := vars["after"].(string); ok && cursor != "" {
		seen[cursor] = true
	}
	for {
		data, err := a.query(ctx, query, vars)
		if err != nil {
			return nil, err
		}
		conn := data
		for _, key := range strings.Split(root, ".") {
			conn, err = asObject(conn[key])
			if err != nil {
				return nil, fmt.Errorf("%s: %w", root, err)
			}
		}
		nodes, err := connectionNodes(conn)
		if err != nil {
			return nil, err
		}
		pi, err := asObject(conn["pageInfo"])
		if err != nil {
			return nil, fmt.Errorf("%s: missing pageInfo", root)
		}
		next, ok := pi["hasNextPage"].(bool)
		if !ok {
			return nil, fmt.Errorf("missing hasNextPage")
		}
		if !a.opts.has("all") {
			return conn, nil
		}
		merged = append(merged, nodes...)
		if len(merged) > 100000 {
			return nil, fmt.Errorf("--all exceeded 100000 records; narrow the filter")
		}
		if !next {
			if merged == nil {
				merged = []any{}
			}
			conn["nodes"] = merged
			return conn, nil
		}
		cursor := stringField(pi, "endCursor")
		if cursor == "" || seen[cursor] {
			return nil, fmt.Errorf("pagination did not advance; refusing to loop")
		}
		seen[cursor] = true
		vars["after"] = cursor
	}
}
func (a *app) issueObject(ctx context.Context, ref string) (object, error) {
	id, err := normalizeIssue(ref, a.opts.get("team"))
	if err != nil {
		return nil, err
	}
	data, err := a.query(ctx, "query ResolveIssue($id: String!) { issue(id: $id) { "+issueSummary+" } }", object{"id": id})
	if err != nil {
		return nil, err
	}
	m, err := asObject(data["issue"])
	if err != nil {
		return nil, fmt.Errorf("issue not found")
	}
	if stringField(m, "id") == "" {
		return nil, fmt.Errorf("issue response has no UUID")
	}
	return m, nil
}
func (a *app) teamID(ctx context.Context, required bool) (string, error) {
	o := a.opts
	if o.supplied("team") && o.supplied("team-id") {
		return "", usageError("choose --team or --team-id, not both")
	}
	if o.supplied("team-id") {
		if !uuidRE.MatchString(o.get("team-id")) {
			return "", usageError("--team-id must be a UUID")
		}
		return o.get("team-id"), nil
	}
	key := o.get("team")
	if key == "" {
		if required {
			return "", usageError("--team KEY or --team-id UUID is required")
		}
		return "", nil
	}
	if !aliasRE.MatchString(key) {
		return "", usageError("invalid team key")
	}
	data, err := a.query(ctx, "query ResolveTeam($filter: TeamFilter) { teams(first: 2, filter: $filter) { nodes { id key name } } }", object{"filter": object{"key": object{"eqIgnoreCase": key}}})
	if err != nil {
		return "", err
	}
	conn, err := asObject(data["teams"])
	if err != nil {
		return "", err
	}
	nodes, err := connectionNodes(conn)
	if err != nil {
		return "", err
	}
	if len(nodes) != 1 {
		return "", usageError("team key %q matched %d teams; supply --team-id UUID", key, len(nodes))
	}
	m, err := asObject(nodes[0])
	if err != nil {
		return "", err
	}
	id := stringField(m, "id")
	if id == "" {
		return "", fmt.Errorf("team response has no id")
	}
	return id, nil
}
func (a *app) stateID(ctx context.Context, teamID, name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", usageError("--state cannot be empty")
	}
	data, err := a.query(ctx, "query ResolveState($filter: WorkflowStateFilter) { workflowStates(first: 2, filter: $filter) { nodes { id name } } }", object{"filter": object{"team": object{"id": object{"eq": teamID}}, "name": object{"eqIgnoreCase": name}}})
	if err != nil {
		return "", err
	}
	conn, err := asObject(data["workflowStates"])
	if err != nil {
		return "", err
	}
	n, err := connectionNodes(conn)
	if err != nil {
		return "", err
	}
	if len(n) != 1 {
		return "", usageError("state %q matched %d states in this team; supply --state-id UUID", name, len(n))
	}
	m, err := asObject(n[0])
	if err != nil {
		return "", err
	}
	return stringField(m, "id"), nil
}
func (a *app) assigneeID(ctx context.Context, email string) (any, error) {
	if email == "none" {
		return nil, nil
	}
	data, err := a.query(ctx, "query ResolveUser($filter: UserFilter) { users(first: 2, filter: $filter) { nodes { id email } } }", object{"filter": object{"email": object{"eqIgnoreCase": email}}})
	if err != nil {
		return nil, err
	}
	conn, err := asObject(data["users"])
	if err != nil {
		return nil, err
	}
	n, err := connectionNodes(conn)
	if err != nil {
		return nil, err
	}
	if len(n) != 1 {
		return nil, usageError("assignee email matched %d users; supply --assignee-id UUID", len(n))
	}
	m, err := asObject(n[0])
	if err != nil {
		return nil, err
	}
	return stringField(m, "id"), nil
}
func uuidValue(value, flag string, nullable bool) (any, error) {
	if value == "none" && nullable {
		return nil, nil
	}
	if !uuidRE.MatchString(value) {
		return nil, usageError("--%s requires a UUID%s", flag, map[bool]string{true: " or none", false: ""}[nullable])
	}
	return value, nil
}
func integerValue(value, flag string, min, max int) (int, error) {
	n, err := strconv.Atoi(value)
	if err != nil || n < min || n > max {
		return 0, usageError("--%s must be %d..%d", flag, min, max)
	}
	return n, nil
}
func parseObject(raw string) (object, error) {
	var v object
	if err := json.Unmarshal([]byte(raw), &v); err != nil || v == nil {
		return nil, usageError("variables must be a JSON object")
	}
	return v, nil
}
