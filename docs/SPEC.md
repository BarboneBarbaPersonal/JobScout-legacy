# JobScout spec: board v2 (2026-10-05)

## 1. Stats bar (top of the page)
Shown on every page build, from the run log and the saved matches:
- Last successful scan (time) and run status
- Scans in the last 24 hours
- Jobs scanned (open postings across all boards) and boards OK (e.g. 129/129)
- New postings in the last scan and in the last 24 hours (all titles)
- Matches on the board: total, Barcelona / Remote / Spain, core / adjacent roles
- New matches in the last 24 hours

Run history is kept in `runs.json` on the `gh-pages` branch (last 500 runs).

## 2. Filters (on the page, no reload)
- Location: All / Barcelona / Remote / Spain
- Role type: All / Core (TAM, CSM, CSE, FDE, SDM...) / Adjacent (everything else)
Column counts update with the filters.

## 3. 30-day rule
- At scan time: a `recency` rule (max 30 days) is added to the matcher. Jobs
  without a posting date still pass.
- On the page: a job leaves the board when its posting date is over 30 days
  old, or (no posting date) 30 days after JobScout first found it.

## 4. Schedule
Cron twice an hour (:13 and :43). GitHub drops some scheduled runs, so this
should land close to one scan per hour.

## 5. More roles (adjacent)
Added to the title rule and the prefilter:
- Account / Client: account manager, client success, client partner, partner success, renewals manager
- Customer Ops / Experience: customer operations, customer experience, escalation manager, customer support manager

## 6. More companies
User adds companies to the Google Sheet; then re-run `jobscout/discover_ats.py`
and add the found boards to `jobscout.yaml`.

## 7. Language focus (scan rule)
Only Italian / English / Spanish speaking roles. Titles naming another
language ("French speaking", "German Speaking") or a market that needs one
("DACH", "Nordics", "Japan", "Benelux") are excluded in the title rule and
the prefilter. Titles only: descriptions often list languages as "nice to
have", so checking them would drop good jobs.

## 8. Rebuild after a full rescan
When the state is reset (bootstrap run), the board is rebuilt from that scan's
matches, so jobs that no longer pass the rules disappear. Jobs that match
again keep their original "found" date.

---

# v3 (2026-10-08): CV proposals

## 9. Market search -- DROPPED (2026-10-08)
Adzuna was considered (free key, personal use allowed) and dropped by the user.
Coverage grows by adding more companies to the Google Sheet instead.

## 10. Like -> CV proposals (private repo `jobscout-cv`)
- Each card gets a **Like** button. It opens a pre-filled new issue in the private
  repo `BarboneBarbaPersonal/jobscout-cv` (title = company + job title, body = job link + JobScout id).
  Nothing about liked jobs or the CV is ever stored in the public JobScout repo.
- `jobscout-cv` holds the master CV as text plus 2-3 base versions (e.g. CSM/TAM, FDE/technical).
- A workflow runs on every new issue:
  1. fetch the job description from the link;
  2. **keyword gap (no AI)**: important terms in the job description missing from the CV;
  3. **Gemma 3 4B via Ollama** on the runner (free, no key): pick the best base version and
     write 3-5 concrete edits (keyword to add, bullet to reword or move up, summary line), plus a fit score;
  4. post the result as a comment on the issue (visible in the GitHub app).
- For important jobs the user asks Claude in a session for a polished version (on request only).
- Later option: run a bigger Gemma on the user's PC (32 GB+ RAM) for higher-quality proposals.

## Needs from the user
- OK to create the private repo `jobscout-cv` and store the CV text there.

---

# v4 (2026-10-09): CV insights inside the dashboard (no GitHub visits, no emails)

## 11. Private access key (per device)
- Dashboard settings: the user pastes a fine-grained GitHub token limited to the private
  repo `jobscout-cv` (Contents: read, Issues: read and write). Stored only in that
  browser's localStorage. Without a key the page looks and works as before (public view).
- With the key the page reads private data straight from `jobscout-cv` via the GitHub API.
  Nothing CV-related is ever written to the public JobScout repo or page.

## 12. Draft for every job (in the browser, no AI)
- The page loads `cv/master.md` and `data/vocabulary.json` from `jobscout-cv` and each job's
  description from the public `matches.json`.
- Each card shows: **match %** (share of the job's important terms found in the CV) and the
  top missing keywords.

## 13. Like -> proposals in the dashboard
- Like creates an issue in `jobscout-cv` through the API (no GitHub page opened).
- The `analyze` workflow (triggered by the new issue) writes `results/<job>.json`:
  1. main missing keywords (no AI);
  2. intro rewrite: 2-3 sentences for the Professional Summary (Gemma 3 4B), fact-checked;
  3. past roles: CV bullets ranked by relevance to the job (no AI): 2 to lead with, 2 to
     shorten; plus up to 3 short Gemma flags on big changes.
- No issue comments (so no notification emails). The card shows "Analyzing..." and then a
  **Proposals** panel, polling the results file every 30 s while open.
- Deeper edits: "CV edits for jobscout-cv issue #N" to Claude in a session (unchanged).

---

# v5 (2026-10-13): Shareable JobScout (template for non-technical users)

## Decisions (from the user)
- Target user: **non-technical** - only GitHub's website, no terminal, no Python/Go.
- Setup: **template + own copy**. `JobScout` becomes a clean template with example
  settings; each user (including Andrea) runs their own copy.
- Companies: **CSV file in the repo** (`companies.csv`), editable on GitHub's website.
- License: jobwatch has no license; **parked for now** (README states personal use).

## Status of coverage (2026-10-13)
166 companies in Andrea's sheet: 154 scanned (152 boards incl. parents), 12 not:
Revolut (blocked from GitHub by a bot check), Sesame (unsolved), and 10 with no
public listing right now (see jobscout/companies.missing.csv).

## Review: what blocks another user today
1. `jobscout.yaml` mixes personal choices with matcher internals; the title list is
   written twice (prefilter + rule) and the country list twice; a missing comma once
   broke matching silently.
2. Personal values hard-coded: repo/page URLs (workflow, page, CV add-on),
   Barcelona/Remote/Spain tiers and core roles in build_page.py.
3. Adding companies needs Python locally (discover_ats.py) and copy-paste.
4. Docs spread over JOBSCOUT.md, SPEC.md and upstream's README.

## Target design
### Two files a user edits (on GitHub's website)
`profile.yaml` - plain, commented:
```yaml
page_title: "My job board"
roles:
  core: [technical account manager, customer success manager, forward deployed engineer]
  also: [account manager, implementation manager, onboarding specialist]
  exclude: [intern, director, vp, solutions engineer]
languages: [english, spanish, italian]      # titles asking for other languages are dropped
locations:                                  # in order of preference
  - Barcelona
  - remote                                  # remote roles open to your region
  - Spain
remote_region: europe                       # which "remote" counts: europe | worldwide | any
max_age_days: 30
```
`companies.csv` - `company,website` (+ optional `board` to force a job board).

### What the template does automatically
- `scripts/build_config.py`: profile.yaml + boards.yaml -> generated jobwatch config
  (prefilter, title rule, language excludes, location rules incl. the built-in list of
  non-target countries/cities) and page settings (tiers, core roles, title). Never edited by hand.
- Workflow **"Find job boards"**: runs when companies.csv changes; detects each company's
  board (discover_ats.py) into `boards.yaml`; lists companies it could not match in the
  run summary.
- Workflow **"Update JobScout"** (manual button): pulls engine updates from the template
  without touching profile.yaml / companies.csv / boards.yaml.
- All repo/page URLs come from GitHub's own variables (owner, repo name), nothing hard-coded.
- CV insights stay an **optional add-on** (separate README section; needs a private repo + key).

### README quick start (target: 10 minutes, browser only)
1. "Use this template" -> create your repo (public, for the free page).
2. Edit `profile.yaml`.
3. Edit `companies.csv`.
4. Settings -> Pages -> deploy from `gh-pages`; Actions -> enable.
5. Actions -> "Find job boards" -> "Start scan". Your page: https://<you>.github.io/<repo>/

## Phases
1. **Settings refactor (no visible change for Andrea):** profile.yaml + companies.csv +
   build_config.py + boards.yaml; generated config must give the same matches as today
   (compare dry-runs). Remove hard-coded URLs.
2. **Automation:** "Find job boards" and "Update JobScout" workflows; run summary of
   unmatched companies.
3. **Split:** JobScout -> clean template (example profile/companies, marked as template);
   Andrea's copy `jobscout-andrea` with his settings, state and page (new page URL;
   CV add-on pointed at it).
4. **Docs:** README quick start + troubleshooting + CV add-on guide; test the quick start
   end-to-end as a new user (fresh copy).

## Open questions / risks
- Template copies don't auto-update -> "Update JobScout" workflow; conflicts only if a
  user edits engine files.
- Free GitHub Pages needs a public repo: a user's companies/roles are visible to anyone
  with the link (state this in the README).
- jobwatch license unresolved (parked).
