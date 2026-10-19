package main

import (
	"context"
	"strings"
)

func (a *app) rawGraphQL(ctx context.Context) error {
	o := a.opts
	if err := o.validate("query-file variables variables-file dry-run"); err != nil {
		return err
	}
	if len(o.Args) > 1 {
		return usageError("graphql takes at most one query argument")
	}
	if len(o.Args) > 0 && o.supplied("query-file") {
		return usageError("choose a query argument or --query-file")
	}
	if o.supplied("variables") && o.supplied("variables-file") {
		return usageError("choose --variables or --variables-file")
	}
	stdinQuery := len(o.Args) == 0 && (!o.supplied("query-file") || o.get("query-file") == "-")
	if stdinQuery && o.get("variables-file") == "-" {
		return usageError("query and variables cannot both read stdin")
	}
	var q string
	var err error
	if len(o.Args) == 1 {
		q = o.Args[0]
	} else if o.supplied("query-file") {
		q, err = a.fileContent(o.get("query-file"))
	} else {
		q, err = a.fileContent("-")
	}
	if err != nil {
		return err
	}
	if strings.TrimSpace(q) == "" {
		return usageError("GraphQL query is empty")
	}
	vars := object{}
	if o.supplied("variables") {
		vars, err = parseObject(o.get("variables"))
	}
	if o.supplied("variables-file") {
		var s string
		s, err = a.fileContent(o.get("variables-file"))
		if err == nil {
			vars, err = parseObject(s)
		}
	}
	if err != nil {
		return err
	}
	if o.has("dry-run") {
		return a.printJSON(object{"dryRun": true, "query": q, "variables": vars})
	}
	data, err := a.query(ctx, q, vars)
	if err != nil {
		return err
	}
	return a.printJSON(data)
}
