package main

import (
	"context"
	"fmt"
)

const listFlags = "first after all include-archived team team-id"

func (a *app) readIssue(ctx context.Context) error {
	o := a.opts
	if err := o.requireArgs(1); err != nil {
		return err
	}
	if err := o.validate("team with-comments"); err != nil {
		return err
	}
	id, err := normalizeIssue(o.Args[0], o.get("team"))
	if err != nil {
		return err
	}
	fields := issueDetail + ` relations(first: 50) { nodes { id type issue { id identifier } relatedIssue { id identifier } } ` + pageFields + ` } inverseRelations(first: 50) { nodes { id type issue { id identifier } relatedIssue { id identifier } } ` + pageFields + ` }`
	if o.has("with-comments") {
		fields += " comments(first: 25) { nodes { " + commentFields + " } " + pageFields + " }"
	}
	data, err := a.query(ctx, "query Issue($id: String!) { issue(id: $id) { "+fields+" } }", object{"id": id})
	if err != nil {
		return err
	}
	issue, err := asObject(data["issue"])
	if err != nil {
		return fmt.Errorf("issue not found")
	}
	return a.printJSON(issue)
}
func (a *app) readComments(ctx context.Context) error {
	o := a.opts
	if err := o.requireArgs(1); err != nil {
		return err
	}
	if err := o.validate("team first after all"); err != nil {
		return err
	}
	id, err := normalizeIssue(o.Args[0], o.get("team"))
	if err != nil {
		return err
	}
	first, err := o.first()
	if err != nil {
		return err
	}
	vars := object{"id": id, "first": first, "after": nil}
	if o.supplied("after") {
		vars["after"] = o.get("after")
	}
	query := "query Comments($id: String!, $first: Int!, $after: String) { issue(id: $id) { comments(first: $first, after: $after) { nodes { " + commentFields + " } " + pageFields + " } } }"
	conn, err := a.paginate(ctx, query, vars, "issue.comments")
	if err != nil {
		return err
	}
	return a.printJSON(conn)
}
func (a *app) readList(ctx context.Context) error {
	o := a.opts
	extras := listFlags
	nArgs := 0
	root, filterType, fields := "", "", ""
	switch o.Command {
	case "issues":
		root = "issues"
		filterType = "IssueFilter"
		fields = issueSummary + " updatedAt"
		extras += " state state-id assignee assignee-id project-id cycle-id"
	case "search":
		root = "searchIssues"
		filterType = "IssueFilter"
		fields = issueSummary
		nArgs = 1
	case "teams":
		root = "teams"
		filterType = "TeamFilter"
		fields = "id key name description"
		extras = "first after all include-archived"
	case "users":
		root = "users"
		filterType = "UserFilter"
		fields = "id name email active"
		extras = "first after all include-archived name email"
	case "states":
		root = "workflowStates"
		filterType = "WorkflowStateFilter"
		fields = "id name type color position team { id key }"
	case "labels":
		root = "issueLabels"
		filterType = "IssueLabelFilter"
		fields = "id name color description team { id key }"
	case "projects":
		root = "projects"
		filterType = "ProjectFilter"
		fields = "id name url state description"
	case "cycles":
		root = "cycles"
		filterType = "CycleFilter"
		fields = "id name number startsAt endsAt completedAt team { id key }"
	}
	if err := o.requireArgs(nArgs); err != nil {
		return err
	}
	if err := o.validate(extras); err != nil {
		return err
	}
	first, err := o.first()
	if err != nil {
		return err
	}
	filter := object{}
	if o.Command != "teams" && o.Command != "users" {
		teamID, err := a.teamID(ctx, o.Command == "states" || o.Command == "cycles")
		if err != nil {
			return err
		}
		if teamID != "" {
			teamFilter := object{"id": object{"eq": teamID}}
			switch o.Command {
			case "projects":
				filter["accessibleTeams"] = object{"some": teamFilter}
			case "labels":
				filter["or"] = []any{object{"team": teamFilter}, object{"team": object{"null": true}}}
			default:
				filter["team"] = teamFilter
			}
		}
	}
	if o.Command == "users" {
		if o.supplied("name") {
			filter["name"] = object{"containsIgnoreCase": o.get("name")}
		}
		if o.supplied("email") {
			filter["email"] = object{"containsIgnoreCase": o.get("email")}
		}
	}
	if o.Command == "issues" {
		if o.supplied("state") && o.supplied("state-id") {
			return usageError("choose --state or --state-id")
		}
		if o.supplied("assignee") && o.supplied("assignee-id") {
			return usageError("choose --assignee or --assignee-id")
		}
		if o.supplied("state") {
			filter["state"] = object{"name": object{"eqIgnoreCase": o.get("state")}}
		}
		if o.supplied("state-id") {
			id, err := uuidValue(o.get("state-id"), "state-id", false)
			if err != nil {
				return err
			}
			filter["state"] = object{"id": object{"eq": id}}
		}
		if o.supplied("assignee") {
			if o.get("assignee") == "none" {
				filter["assignee"] = object{"null": true}
			} else {
				filter["assignee"] = object{"email": object{"eqIgnoreCase": o.get("assignee")}}
			}
		}
		for _, item := range []struct{ flag, field string }{{"assignee-id", "assignee"}, {"project-id", "project"}, {"cycle-id", "cycle"}} {
			if o.supplied(item.flag) {
				id, err := uuidValue(o.get(item.flag), item.flag, true)
				if err != nil {
					return err
				}
				if id == nil {
					filter[item.field] = object{"null": true}
				} else {
					filter[item.field] = object{"id": object{"eq": id}}
				}
			}
		}
	}
	vars := object{"first": first, "after": nil, "filter": filter, "includeArchived": o.has("include-archived")}
	if o.supplied("after") {
		vars["after"] = o.get("after")
	}
	decl := "$first: Int!, $after: String, $filter: " + filterType + ", $includeArchived: Boolean!"
	args := "first: $first, after: $after, filter: $filter, includeArchived: $includeArchived"
	if o.Command == "search" {
		if o.Args[0] == "" {
			return usageError("search text cannot be empty")
		}
		decl += ", $term: String!"
		args += ", term: $term"
		vars["term"] = o.Args[0]
	} else {
		args += ", orderBy: updatedAt"
	}
	query := "query List(" + decl + ") { " + root + "(" + args + ") { nodes { " + fields + " } " + pageFields + " } }"
	conn, err := a.paginate(ctx, query, vars, root)
	if err != nil {
		return err
	}
	return a.printJSON(conn)
}
