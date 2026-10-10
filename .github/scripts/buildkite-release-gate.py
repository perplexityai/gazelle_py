"""Wait for the exact main Buildkite build before releasing a source archive."""

import argparse
import json
import os
import re
import subprocess
import time
import urllib.request


def check_status(statuses, pipeline):
    context = f"release-ready/{pipeline}"
    status = next((item for item in statuses if item["context"] == context), None)
    if status is None:
        return False
    if status["creator"]["login"] != "buildkite[bot]":
        raise ValueError("Release status was not published by Buildkite")
    target = status.get("target_url") or ""
    if not re.fullmatch(rf"https://buildkite\.com/perplexity/{re.escape(pipeline)}/builds/[1-9][0-9]*", target):
        raise ValueError("Unexpected Buildkite release status URL")
    if status["state"] in ("failure", "error"):
        raise ValueError(f"Buildkite release checks failed: {target}")
    if status["state"] == "success":
        print(f"Buildkite release checks passed: {target}")
        return True
    if status["state"] != "pending":
        raise ValueError("Unexpected Buildkite status state")
    return False


def statuses(repo, commit):
    url = f"https://api.github.com/repos/{repo}/commits/{commit}/statuses?per_page=100"
    result = []
    while url:
        request = urllib.request.Request(url, headers={
            "Authorization": f"Bearer {os.environ['GH_TOKEN']}",
            "Accept": "application/vnd.github+json",
            "User-Agent": "buildkite-release-gate",
        })
        with urllib.request.urlopen(request, timeout=30) as response:
            result.extend(json.load(response))
            links = response.headers.get("Link", "")
        match = re.search(r'<([^>]+)>; rel="next"', links)
        url = match.group(1) if match else None
        if url and not url.startswith("https://api.github.com/"):
            raise ValueError("Unexpected GitHub pagination URL")
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--expected-commit")
    # The reusable release workflow appends these flags to its test command.
    parser.add_argument("--disk_cache")
    parser.add_argument("--repository_cache")
    args = parser.parse_args()
    commit = subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip()
    if args.expected_commit:
        if not re.fullmatch(r"[0-9a-f]{40}", args.expected_commit) or commit != args.expected_commit:
            raise ValueError("Release tag moved after Buildkite verification")
        print(f"Packaging verified release commit {commit}; tests ran on Buildkite")
        return
    repo = os.environ["GITHUB_REPOSITORY"]
    pipelines = {
        "perplexityai/gazelle_py": "gazelle-py",
        "perplexityai/gazelle_rs": "gazelle-rs",
        "perplexityai/rules_web_e2e": "rules-web-e2e",
        "perplexityai/gazelle_css": "gazelle-css",
    }
    pipeline = pipelines[repo]
    tag = os.environ["RELEASE_TAG"]
    if not re.fullmatch(r"v[0-9]+\.[0-9]+\.[0-9]+", tag):
        raise ValueError("Expected a stable vX.Y.Z release tag")
    tagged = subprocess.check_output(["git", "rev-parse", f"refs/tags/{tag}^{{commit}}"], text=True).strip()
    if tagged != commit:
        raise ValueError("Checkout does not match release tag")
    subprocess.run(["git", "merge-base", "--is-ancestor", commit, "origin/main"], check=True)
    deadline = time.monotonic() + 90 * 60
    while time.monotonic() < deadline:
        if check_status(statuses(repo, commit), pipeline):
            with open(os.environ["GITHUB_OUTPUT"], "a", encoding="utf-8") as output:
                output.write(f"commit={commit}\n")
            return
        print(f"Waiting for main Buildkite release checks at {commit}", flush=True)
        time.sleep(20)
    raise TimeoutError("Buildkite release checks missing or pending after 90 minutes")


if __name__ == "__main__":
    main()
