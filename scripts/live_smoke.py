#!/usr/bin/env python3
"""Opt-in live CLI test. Creates exactly two marked issues, then archives them.

Never pass tokens here. Use an existing CLI workspace alias. Write permission is
explicit via --allow-write. A failed create is reconciled by its unique title,
not retried. Read-only discovery also checks the second alias if supplied.
"""
import argparse
import datetime as dt
import json
import pathlib
import subprocess
import tempfile
import uuid

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--binary', default='./linear-cli')
parser.add_argument('--workspace', required=True)
parser.add_argument('--team', required=True)
parser.add_argument('--expected-org', required=True)
parser.add_argument('--read-workspace')
parser.add_argument('--allow-write', action='store_true')
args = parser.parse_args()
if not args.allow_write:
    parser.error('live write tests require --allow-write; this creates two issues')

run_id = str(uuid.uuid4())[:12]
started = dt.datetime.now(dt.timezone.utc).isoformat()
report_dir = pathlib.Path('.live-test')
report_dir.mkdir(mode=0o700, exist_ok=True)
report_path = report_dir / f'live-{run_id}.json'
report = {'source_url': 'https://api.linear.app/graphql', 'retrieved_at': started,
          'workspace': args.workspace, 'team': args.team, 'run_id': run_id,
          'steps': [], 'issues': [], 'cleanup': [], 'success': False}
created, planned = [], []
relation_ids, comment_ids = [], []


def persist():
    report_path.write_text(json.dumps(report, indent=2, ensure_ascii=False) + '\n')
    report_path.chmod(0o600)


def step(name, **details):
    report['steps'].append({'name': name, 'retrieved_at': dt.datetime.now(dt.timezone.utc).isoformat(), **details})
    persist()
    print('PASS:', name, flush=True)


def cli(*cmd, workspace=None):
    proc = subprocess.run([args.binary, *cmd, '--workspace', workspace or args.workspace, '--json'],
                          text=True, capture_output=True, timeout=90)
    if proc.returncode:
        raise RuntimeError(f'{cmd[0]} exited {proc.returncode}: {proc.stderr.strip()}')
    return json.loads(proc.stdout)


def issue(ref):
    return cli('issue', ref, '--with-comments')


persist()
failure = None
try:
    ident = cli('doctor')
    assert ident['organization']['urlKey'] == args.expected_org, 'wrong organization; no writes allowed'
    teams = cli('teams', '--all')['nodes']
    assert sum(t['key'] == args.team for t in teams) == 1, 'team not uniquely found'
    step('identity and team verified', organization=ident['organization']['name'])
    if args.read_workspace:
        other = cli('doctor', workspace=args.read_workspace)
        cli('teams', workspace=args.read_workspace)
        step('secondary workspace read-only', alias=args.read_workspace,
             organization=other['organization']['name'])

    states = cli('states', '--team', args.team, '--all')['nodes']
    labels = cli('labels', '--team', args.team, '--all')['nodes']
    for command in ('projects', 'cycles'):
        result = cli(command, '--team', args.team, '--first', '1')
        assert 'nodes' in result and 'pageInfo' in result
    cli('users', '--email', ident['viewer']['email'], '--first', '2')
    step('states labels projects cycles users live reads')

    with tempfile.TemporaryDirectory(prefix='linear-cli-smoke-') as temp:
        desc = 'CLI smoke-test fixture only. Safe to archive.\n\nUnicode: 日本語 Indonesia.\n\n```go\nfmt.Println("quotes", `backticks`, "C:\\\\tmp")\n```\n'
        desc_path = pathlib.Path(temp) / 'description.md'
        desc_path.write_text(desc)
        dry = cli('create', '--team', args.team, '--title', f'[linear-cli smoke test] {run_id} dry',
                  '--desc-file', str(desc_path), '--dry-run')
        assert dry['dryRun'] is True and dry['variables']['input']['description'] == desc
        step('create dry-run preserves content')

        for suffix in ('A', 'B'):
            title = f'[linear-cli smoke test] {run_id} {suffix}'
            planned.append(title)
            persist()
            made = cli('create', '--team', args.team, '--title', title,
                       '--desc-file', str(desc_path), '--priority', '0')
            assert made['success'] is True
            obj = made['issue']
            created.append(obj['id'])
            report['issues'].append({k: obj[k] for k in ('id', 'identifier', 'url')})
            persist()
            read = issue(obj['id'])
            assert read['title'] == title and '日本語' in read['description'] and 'fmt.Println' in read['description']
            step('created and re-read fixture ' + suffix, identifier=obj['identifier'])

        a, b = created
        state = next((s for s in states if s['type'] in ('backlog', 'unstarted')), states[0])
        cli('update', a, '--title', f'[linear-cli smoke test] {run_id} A updated', '--state', state['name'],
            '--priority', '3', '--assignee', ident['viewer']['email'])
        read = issue(a)
        assert read['priority'] == 3 and read['state']['id'] == state['id']
        assert read['assignee']['id'] == ident['viewer']['id']
        step('updated title priority team-scoped state and assignee email')
        cli('update', a, '--assignee-id', 'none', '--due-date', 'none')
        assert issue(a)['assignee'] is None
        if labels:
            label_id = labels[0]['id']
            cli('update', a, '--label-ids', label_id)
            assert label_id in {x['id'] for x in issue(a)['labels']['nodes']}
            cli('update', a, '--label-ids', '')
            assert not issue(a)['labels']['nodes']
            step('set and explicitly cleared label set')

        body = 'Temporary CLI test comment.\n\nQuotes: "hello", backslash \\, Unicode 日本語.\n'
        body_path = pathlib.Path(temp) / 'comment.md'
        body_path.write_text(body)
        comment = cli('comment', a, '--body-file', str(body_path))['comment']
        comment_ids.append(comment['id'])
        assert '日本語' in comment['body']
        revised = body + '\nEdited by smoke test.\n'
        body_path.write_text(revised)
        cli('update-comment', comment['id'], '--body-file', str(body_path))
        comments = cli('comments', a, '--all', '--first', '1')['nodes']
        assert any(c['id'] == comment['id'] and 'Edited by smoke test.' in c['body'] for c in comments)
        cli('delete-comment', comment['id'], '--yes')
        comment_ids.remove(comment['id'])
        assert not any(c['id'] == comment['id'] for c in cli('comments', a, '--all')['nodes'])
        step('created edited paginated re-read and deleted own comment')

        for kind in ('blocks', 'blockedBy'):
            rel = cli('relate', a, b, '--type', kind)['issueRelation']
            relation_ids.append(rel['id'])
            expected_left, expected_right = (a, b) if kind == 'blocks' else (b, a)
            assert rel['issue']['id'] == expected_left and rel['relatedIssue']['id'] == expected_right
            read = issue(a)
            all_relations = read['relations']['nodes'] + read['inverseRelations']['nodes']
            assert any(r['id'] == rel['id'] for r in all_relations)
            cli('unrelate', rel['id'], '--yes')
            relation_ids.remove(rel['id'])
            step('created verified and removed ' + kind + ' relation')

        cli('relate', a, b, '--type', 'parent')
        assert issue(b)['parent']['id'] == a
        cli('update', b, '--parent-id', 'none')
        assert issue(b)['parent'] is None
        step('created actual sub-issue and detached parent')

        raw = cli('graphql', 'query Smoke($id: String!) { issue(id: $id) { id identifier title } }',
                  '--variables', json.dumps({'id': a}))
        assert raw['issue']['id'] == a
        step('raw GraphQL variables round-trip')
        cli('issues', '--team', args.team, '--first', '1')
        found = cli('search', run_id, '--team', args.team, '--all')['nodes']
        # Full-text indexing is asynchronous; identifier reads are authoritative.
        step('filtered list and search execute', search_count=len(found),
             note='Search indexing can lag newly created issues.')

        cli('archive', a, '--yes')
        assert issue(a)['archivedAt'] is not None
        cli('unarchive', a, '--yes')
        assert issue(a)['archivedAt'] is None
        step('archive and unarchive verified')
    report['success'] = True
except Exception as exc:
    failure = str(exc)
    report['failure'] = failure
finally:
    # A timed-out create may have committed remotely. Reconcile only exact,
    # unique fixture titles; never replay the mutation or touch unrelated issues.
    if planned:
        try:
            data = cli('graphql',
                       'query Recover($titles: [String!]!) { issues(first: 10, includeArchived: true, filter: {title: {in: $titles}}) { nodes { id identifier title url } } }',
                       '--variables', json.dumps({'titles': planned}))
            for obj in data['issues']['nodes']:
                if obj['title'] in planned and obj['id'] not in created:
                    created.append(obj['id'])
                    report['issues'].append({k: obj[k] for k in ('id', 'identifier', 'url')})
        except Exception as exc:
            report['reconciliation_error'] = str(exc)
    for comment_id in list(comment_ids):
        try:
            cli('delete-comment', comment_id, '--yes')
        except Exception as exc:
            report['cleanup'].append({'comment_id': comment_id, 'error': str(exc)})
    for relation_id in list(relation_ids):
        try:
            cli('unrelate', relation_id, '--yes')
        except Exception as exc:
            report['cleanup'].append({'relation_id': relation_id, 'error': str(exc)})
    for obj_id in created:
        try:
            current = issue(obj_id)
            if not current['archivedAt']:
                cli('archive', obj_id, '--yes')
            current = issue(obj_id)
            assert current['archivedAt'] is not None
            report['cleanup'].append({'id': obj_id, 'identifier': current['identifier'],
                                      'archivedAt': current['archivedAt'], 'verified': True})
        except Exception as exc:
            report['cleanup'].append({'id': obj_id, 'error': str(exc), 'verified': False})
    if any('error' in c for c in report['cleanup']) or report.get('reconciliation_error'):
        report['success'] = False
    report['finished_at'] = dt.datetime.now(dt.timezone.utc).isoformat()
    persist()
    print('Report:', report_path, flush=True)
    print('Archived fixtures:', ', '.join(c.get('identifier', '?') for c in report['cleanup'] if c.get('verified')), flush=True)
if not report['success']:
    raise SystemExit('FAIL: ' + (failure or 'cleanup/reconciliation failed; inspect report'))
print('PASS: all live checks and archival cleanup', flush=True)
