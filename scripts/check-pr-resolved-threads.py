#!/usr/bin/env python3
import json
import os
import sys
import urllib.error
import urllib.request


API_URL = "https://api.github.com/graphql"


def fail(message: str) -> None:
    print(f"release gate failed: {message}", file=sys.stderr)
    sys.exit(1)


def graphql(query: str, variables: dict) -> dict:
    token = os.environ.get("GH_TOKEN") or os.environ.get("GITHUB_TOKEN")
    if not token:
        fail("GH_TOKEN or GITHUB_TOKEN is required")

    payload = json.dumps({"query": query, "variables": variables}).encode("utf-8")
    request = urllib.request.Request(
        API_URL,
        data=payload,
        method="POST",
        headers={
            "Accept": "application/vnd.github+json",
            "Authorization": f"Bearer {token}",
            "Content-Type": "application/json",
            "User-Agent": "fwlog-release-gate",
            "X-GitHub-Api-Version": "2022-11-28",
        },
    )
    try:
        with urllib.request.urlopen(request) as response:
            body = json.loads(response.read().decode("utf-8"))
    except urllib.error.HTTPError as error:
        detail = error.read().decode("utf-8", errors="replace")
        fail(f"GitHub GraphQL request failed: HTTP {error.code}: {detail}")

    if body.get("errors"):
        fail(f"GitHub GraphQL returned errors: {body['errors']}")
    return body["data"]


def repository() -> tuple[str, str]:
    repo = os.environ.get("GITHUB_REPOSITORY", "")
    if "/" not in repo:
        fail("GITHUB_REPOSITORY must be owner/repo")
    return tuple(repo.split("/", 1))


def associated_pr_number(owner: str, repo: str, commit_sha: str) -> int:
    query = """
    query($owner: String!, $repo: String!, $oid: GitObjectID!) {
      repository(owner: $owner, name: $repo) {
        object(oid: $oid) {
          ... on Commit {
            associatedPullRequests(first: 10) {
              nodes {
                number
                merged
              }
            }
          }
        }
      }
    }
    """
    data = graphql(query, {"owner": owner, "repo": repo, "oid": commit_sha})
    obj = data.get("repository", {}).get("object")
    if not obj:
        fail(f"no commit found for {commit_sha}")

    prs = obj.get("associatedPullRequests", {}).get("nodes", [])
    if not prs:
        fail(f"commit {commit_sha} is not associated with a pull request")

    merged = [pr for pr in prs if pr.get("merged")]
    return int((merged or prs)[0]["number"])


def resolved_thread_count(owner: str, repo: str, pr_number: int) -> int:
    query = """
    query($owner: String!, $repo: String!, $number: Int!, $cursor: String) {
      repository(owner: $owner, name: $repo) {
        pullRequest(number: $number) {
          reviewThreads(first: 100, after: $cursor) {
            nodes {
              isResolved
            }
            pageInfo {
              hasNextPage
              endCursor
            }
          }
        }
      }
    }
    """

    total = 0
    cursor = None
    while True:
        data = graphql(
            query,
            {"owner": owner, "repo": repo, "number": pr_number, "cursor": cursor},
        )
        pr = data.get("repository", {}).get("pullRequest")
        if not pr:
            fail(f"pull request #{pr_number} was not found")

        threads = pr["reviewThreads"]
        total += sum(1 for thread in threads["nodes"] if thread.get("isResolved"))
        page_info = threads["pageInfo"]
        if not page_info["hasNextPage"]:
            return total
        cursor = page_info["endCursor"]


def main() -> None:
    min_resolved = int(os.environ.get("MIN_RESOLVED_THREADS", "5"))
    owner, repo = repository()

    pr_number_raw = os.environ.get("PR_NUMBER", "").strip()
    if pr_number_raw:
        pr_number = int(pr_number_raw)
    else:
        commit_sha = os.environ.get("COMMIT_SHA") or os.environ.get("GITHUB_SHA")
        if not commit_sha:
            fail("PR_NUMBER or COMMIT_SHA/GITHUB_SHA is required")
        pr_number = associated_pr_number(owner, repo, commit_sha)

    resolved = resolved_thread_count(owner, repo, pr_number)
    print(f"PR #{pr_number} has {resolved} resolved review thread(s); required: {min_resolved}")
    if resolved < min_resolved:
        fail(
            f"PR #{pr_number} needs at least {min_resolved} resolved review threads before release"
        )


if __name__ == "__main__":
    main()
