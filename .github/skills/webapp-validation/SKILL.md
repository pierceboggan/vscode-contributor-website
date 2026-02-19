---
name: webapp-validation
description: 'Validate web app functionality by running the server and using Playwright to navigate pages, click elements, and verify features work end-to-end. Use after implementing a feature, fixing a bug, or when the user asks to verify, test, or validate the web app.'
argument-hint: 'Describe the feature or behavior to validate'
---

# Web App Validation

Validate that the Go web app actually works by starting the server, launching Playwright against it, and verifying that pages render correctly and interactive features behave as expected. **Do not stop until the feature genuinely works end-to-end.**

## When to Use

- After implementing or modifying a feature
- After fixing a bug
- When the user asks to "verify", "check", "test", or "make sure it works"
- When you want to confirm a change didn't break existing pages

## Project Context

This is a Go web app served on `localhost:8080` with these routes:

| Route | Description |
|-------|-------------|
| `/` | Home page |
| `/about` | About page |
| `/contributors` | Contributors list (with version selector) |
| `/api/kudos/{user}` | Kudos API (GET/POST) |
| `/static/` | Static assets (CSS) |

**Start command:** `go run main.go`

## Procedure

### 1. Start the Server

Run the Go server in the background:

```sh
go run main.go &
SERVER_PID=$!
```

Wait briefly, then confirm it's listening:

```sh
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080
```

If it fails, check for port conflicts (`lsof -i :8080`) and fix before continuing.

### 2. Install Playwright (if needed)

Check if Playwright is available. If not, install it:

```sh
npx playwright --version || npm init -y && npm install playwright
npx playwright install chromium
```

### 3. Write a Validation Script

Create a temporary Playwright script at `.github/skills/webapp-validation/scripts/validate.mjs` tailored to the specific feature being validated. Use the template in [./scripts/validate-template.mjs](./scripts/validate-template.mjs) as a starting point.

The script must:
- Navigate to the relevant page(s)
- Verify visible text, elements, or behavior that proves the feature works
- Take screenshots on both success and failure (save to `./screenshots/`)
- Exit with code 0 on success, 1 on failure, printing clear pass/fail messages

**Adapt the script to the specific feature.** For example:
- New page → navigate to it, check heading and content render
- New button → find it, click it, assert the expected result
- Style change → screenshot and verify element is visible
- API feature → make fetch calls and check responses

### 4. Run the Validation

```sh
node .github/skills/webapp-validation/scripts/validate.mjs
```

### 5. Interpret Results and Iterate

- **If tests pass:** Report success. Show the user what was verified.
- **If tests fail:** Read the error output and screenshots carefully. **Fix the code**, then re-run validation. Repeat until all checks pass. Do NOT report success until the validation script exits cleanly.

### 6. Clean Up

Stop the background server:

```sh
kill $SERVER_PID 2>/dev/null
```

## Key Principles

1. **Actually run it.** Never assume code works — prove it by running the app and verifying with a real browser.
2. **Be specific.** Tailor the Playwright checks to the exact feature the user asked for. Generic "page loads" is insufficient if the user asked for a working dropdown.
3. **Iterate until done.** If validation fails, fix the underlying issue and re-validate. The loop is: implement → validate → fix → validate → ... → pass.
4. **Screenshot evidence.** Always capture screenshots so the user can see the result.
5. **Don't leave the server running.** Clean up background processes when finished.
