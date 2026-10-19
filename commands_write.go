package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

const issueWriteFlags = "team team-id title desc desc-file priority state state-id assignee assignee-id label-ids project-id cycle-id parent-id due-date dry-run"

func (a *app) fileContent(p string) (string, error) {
	var reader io.Reader = a.in
	if p != "-" {
		file, err := os.Open(expandHome(p, a.home))
		if err != nil {
			return "", fmt.Errorf("read content file: %w", err)
		}
		defer file.Close()
		reader = file
	}
	b, err := io.ReadAll(io.LimitReader(reader, (8<<20)+1))
	if err != nil {
		return "", fmt.Errorf("read content: %w", err)
	}
	if len(b) > 8<<20 {
		return "", usageError("content exceeds 8 MiB")
	}
	return string(b), nil
}
func (a *app) textInput(inline, file string, required bool) (string, bool, error) {
	o := a.opts
	if o.supplied(inline) && o.supplied(file) {
		return "", false, usageError("choose --%s or --%s, not both", inline, file)
	}
	if o.supplied(inline) {
		return o.get(inline), true, nil
	}
	if o.supplied(file) {
		s, e := a.fileContent(o.get(file))
		return s, true, e
	}
	if required {
		return "", false, usageError("--%s or --%s is required", inline, file)
	}
	return "", false, nil
}
func (a *app) mutation(ctx context.Context, query string, vars object, root string) error {
	if a.opts.has("dry-run") {
		return a.printJSON(object{"dryRun": true, "query": query, "variables": vars})
	}
	data, err := a.query(ctx, query, vars)
	if err != nil {
		return err
	}
	payload, err := asObject(data[root])
	if err != nil {
		return fmt.Errorf("%s: missing mutation result", root)
	}
	success, ok := payload["success"].(bool)
	if !ok || !success {
		return fmt.Errorf("%s returned success=false or omitted success; verify current state before retrying", root)
	}
	return a.printJSON(payload)
}
func parseLabelIDs(s string) ([]string, error) {
	labels := []string{}
	if strings.TrimSpace(s) == "" {
		return labels, nil
	}
	if strings.HasPrefix(strings.TrimSpace(s), "[") {
		if err := json.Unmarshal([]byte(s), &labels); err != nil || labels == nil {
			return nil, usageError("--label-ids requires comma-separated UUIDs or a JSON string array")
		}
	} else {
		labels = strings.Split(s, ",")
	}
	for i, id := range labels {
		id = strings.TrimSpace(id)
		if !uuidRE.MatchString(id) {
			return nil, usageError("--label-ids contains a non-UUID value")
		}
		labels[i] = id
	}
	return labels, nil
}

func (a *app) writeIssue(ctx context.Context) error {
	o := a.opts
	create := o.Command == "create"
	wantArgs := 1
	if create {
		wantArgs = 0
	}
	if err := o.requireArgs(wantArgs); err != nil {
		return err
	}
	if err := o.validate(issueWriteFlags); err != nil {
		return err
	}
	if o.supplied("state") && o.supplied("state-id") {
		return usageError("choose --state or --state-id")
	}
	if o.supplied("assignee") && o.supplied("assignee-id") {
		return usageError("choose --assignee or --assignee-id")
	}
	input := object{}
	if o.supplied("title") {
		if strings.TrimSpace(o.get("title")) == "" {
			return usageError("--title cannot be empty")
		}
		input["title"] = o.get("title")
	} else if create {
		return usageError("create requires --title")
	}
	desc, present, err := a.textInput("desc", "desc-file", false)
	if err != nil {
		return err
	}
	if present {
		input["description"] = desc
	}
	if o.supplied("priority") {
		n, err := integerValue(o.get("priority"), "priority", 0, 4)
		if err != nil {
			return err
		}
		input["priority"] = n
	}
	for _, item := range []struct{ flag, field string }{{"assignee-id", "assigneeId"}, {"state-id", "stateId"}, {"project-id", "projectId"}, {"cycle-id", "cycleId"}} {
		if o.supplied(item.flag) {
			v, err := uuidValue(o.get(item.flag), item.flag, item.flag != "state-id")
			if err != nil {
				return err
			}
			input[item.field] = v
		}
	}
	if o.supplied("label-ids") {
		ids, err := parseLabelIDs(o.get("label-ids"))
		if err != nil {
			return err
		}
		input["labelIds"] = ids
	}
	if o.supplied("due-date") {
		s := o.get("due-date")
		if s == "none" {
			input["dueDate"] = nil
		} else {
			if _, err := time.Parse("2006-01-02", s); err != nil {
				return usageError("--due-date requires YYYY-MM-DD or none")
			}
			input["dueDate"] = s
		}
	}
	if !create && len(input) == 0 && !o.supplied("state") && !o.supplied("assignee") && !o.supplied("parent-id") {
		return usageError("update requires at least one issue field")
	}
	teamID, err := a.teamID(ctx, create)
	if err != nil {
		return err
	}
	var id string
	if !create {
		issue, err := a.issueObject(ctx, o.Args[0])
		if err != nil {
			return err
		}
		id = stringField(issue, "id")
		team, err := asObject(issue["team"])
		if err != nil {
			return err
		}
		actualTeam := stringField(team, "id")
		if teamID != "" && actualTeam != teamID {
			return usageError("issue is outside the selected team; refusing update")
		}
		teamID = actualTeam
	}
	if o.supplied("state") {
		id, err := a.stateID(ctx, teamID, o.get("state"))
		if err != nil {
			return err
		}
		input["stateId"] = id
	}
	if o.supplied("assignee") {
		id, err := a.assigneeID(ctx, o.get("assignee"))
		if err != nil {
			return err
		}
		input["assigneeId"] = id
	}
	if o.supplied("parent-id") {
		if o.get("parent-id") == "none" {
			input["parentId"] = nil
		} else {
			parent, err := a.issueObject(ctx, o.get("parent-id"))
			if err != nil {
				return err
			}
			parentID := stringField(parent, "id")
			if !create && parentID == id {
				return usageError("issue cannot be its own parent")
			}
			input["parentId"] = parentID
		}
	}
	if create {
		input["teamId"] = teamID
		q := "mutation CreateIssue($input: IssueCreateInput!) { issueCreate(input: $input) { success issue { " + issueDetail + " } } }"
		return a.mutation(ctx, q, object{"input": input}, "issueCreate")
	}
	q := "mutation UpdateIssue($id: String!, $input: IssueUpdateInput!) { issueUpdate(id: $id, input: $input) { success issue { " + issueDetail + " } } }"
	return a.mutation(ctx, q, object{"id": id, "input": input}, "issueUpdate")
}

func (a *app) writeComment(ctx context.Context) error {
	o := a.opts
	if err := o.requireArgs(1); err != nil {
		return err
	}
	if o.Command == "delete-comment" {
		if err := o.validate("yes dry-run"); err != nil {
			return err
		}
		if !uuidRE.MatchString(o.Args[0]) {
			return usageError("delete-comment requires a full comment UUID")
		}
		if err := o.confirmed(); err != nil {
			return err
		}
		return a.mutation(ctx, "mutation DeleteComment($id: String!) { commentDelete(id: $id) { success } }", object{"id": o.Args[0]}, "commentDelete")
	}
	extra := "body body-file dry-run"
	if o.Command == "comment" {
		extra += " team"
	}
	if err := o.validate(extra); err != nil {
		return err
	}
	body, _, err := a.textInput("body", "body-file", true)
	if err != nil {
		return err
	}
	if strings.TrimSpace(body) == "" {
		return usageError("comment body cannot be empty")
	}
	if o.Command == "update-comment" {
		if !uuidRE.MatchString(o.Args[0]) {
			return usageError("update-comment requires a full comment UUID")
		}
		q := "mutation UpdateComment($id: String!, $input: CommentUpdateInput!) { commentUpdate(id: $id, input: $input) { success comment { " + commentFields + " } } }"
		return a.mutation(ctx, q, object{"id": o.Args[0], "input": object{"body": body}}, "commentUpdate")
	}
	issue, err := a.issueObject(ctx, o.Args[0])
	if err != nil {
		return err
	}
	q := "mutation CreateComment($input: CommentCreateInput!) { commentCreate(input: $input) { success comment { " + commentFields + " } } }"
	return a.mutation(ctx, q, object{"input": object{"issueId": stringField(issue, "id"), "body": body}}, "commentCreate")
}
func (a *app) writeRelation(ctx context.Context) error {
	o := a.opts
	if o.Command == "unrelate" {
		if err := o.requireArgs(1); err != nil {
			return err
		}
		if err := o.validate("yes dry-run"); err != nil {
			return err
		}
		if !uuidRE.MatchString(o.Args[0]) {
			return usageError("unrelate requires a full relationship UUID")
		}
		if err := o.confirmed(); err != nil {
			return err
		}
		return a.mutation(ctx, "mutation DeleteRelation($id: String!) { issueRelationDelete(id: $id) { success } }", object{"id": o.Args[0]}, "issueRelationDelete")
	}
	if err := o.requireArgs(2); err != nil {
		return err
	}
	if err := o.validate("team type dry-run"); err != nil {
		return err
	}
	kind := o.get("type")
	if kind == "" {
		kind = "related"
	}
	switch kind {
	case "blocks", "blockedBy", "related", "duplicate", "parent", "child":
	default:
		return usageError("invalid relation type; choose blocks, blockedBy, related, duplicate, parent, child")
	}
	left, err := a.issueObject(ctx, o.Args[0])
	if err != nil {
		return err
	}
	right, err := a.issueObject(ctx, o.Args[1])
	if err != nil {
		return err
	}
	l, r := stringField(left, "id"), stringField(right, "id")
	if l == r {
		return usageError("cannot relate an issue to itself")
	}
	if kind == "parent" || kind == "child" {
		child, parent := l, r
		if kind == "parent" {
			child, parent = r, l
		}
		q := "mutation SetParent($id: String!, $input: IssueUpdateInput!) { issueUpdate(id: $id, input: $input) { success issue { id identifier parent { id identifier } } } }"
		return a.mutation(ctx, q, object{"id": child, "input": object{"parentId": parent}}, "issueUpdate")
	}
	if kind == "blockedBy" {
		l, r = r, l
		kind = "blocks"
	}
	q := "mutation CreateRelation($input: IssueRelationCreateInput!) { issueRelationCreate(input: $input) { success issueRelation { id type issue { id identifier } relatedIssue { id identifier } } } }"
	return a.mutation(ctx, q, object{"input": object{"issueId": l, "relatedIssueId": r, "type": kind}}, "issueRelationCreate")
}
func (a *app) archiveIssue(ctx context.Context) error {
	o := a.opts
	if err := o.requireArgs(1); err != nil {
		return err
	}
	if err := o.validate("team yes dry-run"); err != nil {
		return err
	}
	if err := o.confirmed(); err != nil {
		return err
	}
	id, err := normalizeIssue(o.Args[0], o.get("team"))
	if err != nil {
		return err
	}
	root := "issueArchive"
	if o.Command == "unarchive" {
		root = "issueUnarchive"
	}
	q := "mutation ArchiveIssue($id: String!) { " + root + "(id: $id) { success } }"
	return a.mutation(ctx, q, object{"id": id}, root)
}
