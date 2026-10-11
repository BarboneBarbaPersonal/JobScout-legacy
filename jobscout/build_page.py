"""
Build the JobScout job board web page.

Inputs:
  new_matches.jsonl      new matches from this run (written by the jsonfile notifier)
  <site>/matches.json    every match still on the board (kept on the gh-pages branch)
  <site>/runs.json       history of scans, for the stats bar
  run.log                (optional) this run's jobwatch log, for scan stats

Outputs:
  <site>/matches.json, <site>/runs.json, <site>/index.html

Run:  python jobscout/build_page.py new_matches.jsonl site [run.log] [rebuild]
      "rebuild" (after a full rescan) replaces the board with this scan's matches.
"""

import json
import re
import sys
from datetime import datetime, timedelta, timezone
from pathlib import Path

NEW_FOR_HOURS = 48   # a job counts as "New" for this long after we first saw it
MAX_AGE_DAYS = 30    # jobs posted (or, without a date, found) longer ago leave the board
KEEP_RUNS = 500      # scans remembered for the stats bar

# Location preference: earlier = better. First tier with a matching term wins.
LOCATION_TIERS = [
    (3, "Barcelona", ["barcelona", "catalunya", "cataluña", "catalonia"]),
    (2, "Remote", ["remote", "distributed", "worldwide", "anywhere", "home based"]),
    (1, "Spain", ["spain", "españa", "espana", "madrid"]),
]
# Role types: core target roles score higher; account roles get their own filter;
# everything else the title rule lets in is "adjacent".
CORE_ROLES = ["technical account manager", "tam", "customer success manager", "csm",
              "customer success engineer", "cse", "forward deployed", "fde",
              "service delivery manager", "sdm", "csam", "customer success architect"]


def has_word(text, term):
    """Whole-word, case-insensitive check ("tam" must not match "team")."""
    text = f" {text.lower()} "
    for separator in ",;:()[]/|-–—.":
        text = text.replace(separator, " ")
    return f" {term} " in text


def location_tier(location):
    for score, label, terms in LOCATION_TIERS:
        if any(has_word(location, term) for term in terms):
            return score, label
    return 0, "Other"


ACCOUNT_ROLES = ["account manager", "client partner", "client success", "renewals manager",
                 "partner success"]


def is_core(title):
    return any(has_word(title, role) for role in CORE_ROLES)


def role_type(title):
    """core / account / adjacent. "Technical Account Manager" counts as core."""
    if is_core(title):
        return "core"
    if any(has_word(title, role) for role in ACCOUNT_ROLES):
        return "account"
    return "adjacent"


def fit_score(job):
    """0-8: location counts double (where matters most), core role adds 2, adjacent 1."""
    location_points, _ = location_tier(job["location"])
    return location_points * 2 + (2 if is_core(job["title"]) else 1)


def parse_time(value):
    return datetime.fromisoformat(value.replace("Z", "+00:00"))


def read_json(path, default):
    return json.loads(path.read_text(encoding="utf-8")) if path.exists() else default


def still_fresh(job, now):
    """30-day rule: use the posting date when the board gives one,
    otherwise the date JobScout first found the job."""
    reference = job.get("posted_at") or job["found_at"]
    return now - parse_time(reference) <= timedelta(days=MAX_AGE_DAYS)


def run_record(log_path, new_match_count, now):
    """Summarize this run's log: BOARD lines give per-board counts, the RUN line the status."""
    if log_path is None or not log_path.exists():
        return None
    boards = boards_ok = scanned = new_postings = 0
    status = "unknown"
    for line in log_path.read_text(encoding="utf-8", errors="replace").splitlines():
        fields = dict(re.findall(r'(\w+)=("[^"]*"|\S+)', line))
        if " BOARD " in line:
            boards += 1
            # "partial" / "capped" / "recovered" boards still returned their jobs
            # (e.g. one malformed posting skipped); only "failed" means nothing loaded.
            boards_ok += fields.get("status") != "failed"
            scanned += int(fields.get("open", 0))
            new_postings += int(fields.get("new", 0))
        elif " RUN " in line:
            status = fields.get("status", status)
    return {"time": now.isoformat(), "status": status, "boards": boards, "boards_ok": boards_ok,
            "scanned": scanned, "new_postings": new_postings, "new_matches": new_match_count}


def build_stats(jobs, runs, now):
    day_ago = now - timedelta(hours=24)
    recent_runs = [run for run in runs if parse_time(run["time"]) >= day_ago]
    good_runs = [run for run in runs if run["status"] in ("ok", "degraded") and run["boards"]]
    last = good_runs[-1] if good_runs else None
    places = [location_tier(job["location"])[1] for job in jobs]
    return {
        "updated": now.isoformat(),
        "last_scan": last["time"] if last else None,
        "last_status": runs[-1]["status"] if runs else None,
        "scans_24h": len(recent_runs),
        "scanned": last["scanned"] if last else None,
        "boards": last["boards"] if last else None,
        "boards_ok": last["boards_ok"] if last else None,
        "new_postings_last": last["new_postings"] if last else None,
        "new_postings_24h": sum(run["new_postings"] for run in recent_runs),
        "new_matches_24h": sum(1 for job in jobs if parse_time(job["found_at"]) >= day_ago),
        "total": len(jobs),
        "barcelona": places.count("Barcelona"),
        "remote": places.count("Remote"),
        "spain": places.count("Spain"),
        "core": sum(1 for job in jobs if role_type(job["title"]) == "core"),
        "account": sum(1 for job in jobs if role_type(job["title"]) == "account"),
    }


def main():
    new_matches_path, site = Path(sys.argv[1]), Path(sys.argv[2])
    log_path = Path(sys.argv[3]) if len(sys.argv) > 3 else None
    rebuild = len(sys.argv) > 4 and sys.argv[4] == "rebuild"
    site.mkdir(parents=True, exist_ok=True)
    now = datetime.now(timezone.utc)

    # Merge this run's matches. A job we already know keeps its first found_at.
    old_jobs = {job["id"]: job for job in read_json(site / "matches.json", [])}
    new_lines = []
    if new_matches_path.exists():
        new_lines = [json.loads(line) for line in new_matches_path.read_text(encoding="utf-8").splitlines() if line.strip()]
    if rebuild:
        # Full rescan: the board becomes exactly this scan's matches (so jobs
        # that no longer pass the rules disappear), keeping old found dates.
        jobs = {}
        for job in new_lines:
            # Fresh data from this scan (e.g. the description), old "found" date.
            old = old_jobs.get(job["id"])
            jobs[job["id"]] = {**job, "found_at": old["found_at"]} if old else job
    else:
        jobs = old_jobs
        for job in new_lines:
            jobs.setdefault(job["id"], job)
    kept = [job for job in jobs.values() if still_fresh(job, now)]

    # Remember this scan for the stats bar.
    runs = read_json(site / "runs.json", [])
    record = run_record(log_path, len(new_lines), now)
    if record and rebuild:
        # A full rescan sees every open job as "new"; that is not a real count.
        record["new_postings"] = 0
        # Marks a full refresh; the workflow uses it to schedule the weekly one.
        record["rebuild"] = True
    if record:
        runs = (runs + [record])[-KEEP_RUNS:]

    # What the page needs per job: the raw fields plus place, role type and fit.
    page_jobs = []
    for job in kept:
        # Descriptions stay in matches.json (for the CV analysis) but not in the page itself.
        page_job = {key: value for key, value in job.items() if key != "description"}
        page_jobs.append({**page_job, "place": location_tier(job["location"])[1],
                          "role": role_type(job["title"]),
                          "fit": fit_score(job)})
    data = {"jobs": page_jobs, "stats": build_stats(kept, runs, now),
            "new_for_hours": NEW_FOR_HOURS, "max_age_days": MAX_AGE_DAYS}

    (site / "matches.json").write_text(json.dumps(kept, indent=1, ensure_ascii=False), encoding="utf-8")
    (site / "runs.json").write_text(json.dumps(runs, indent=1), encoding="utf-8")
    template = (Path(__file__).parent / "page_template.html").read_text(encoding="utf-8")
    # Embedded as JSON; "</" is escaped so job text can never close the <script> tag.
    embedded = json.dumps(data, ensure_ascii=False).replace("</", "<\\/")
    (site / "index.html").write_text(template.replace("{{DATA}}", embedded), encoding="utf-8")
    print(f"board: {len(kept)} jobs ({len(jobs) - len(kept)} dropped by the {MAX_AGE_DAYS}-day rule)")


if __name__ == "__main__":
    main()
