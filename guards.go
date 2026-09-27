package main

// The flags a profile can set in its settings.json: every on/off setting and
// variable Claude Code documents, and the tools a profile can deny. The window
// lists each by name with a box ticked for on. A few start the other way from
// Claude Code's own default: telemetry off, nothing going to claude.ai or
// telling what the sessions are about, and Claude's name out of git; these are
// written out whatever Claude Code's default. A profile keeps only the flags it
// turns the other way from where they start.

import (
	"slices"
	"strings"
)

// A guard is one flag: a setting (dots make a path), a variable in the env
// block, or a tool, which permissions.deny blocks when its box is unticked.
type guard struct {
	Kind   string // "setting", "env" or "deny"
	Key    string
	Ticked bool // where its box starts
	// Known says whether Claude Code's own value, Default, is known. A flag
	// whose box matches it is left out of the file; one with no known default
	// is always written.
	Known, Default bool
	On, Off        any // the value written for a ticked and an unticked box; nil writes nothing
	Desc           string
}

// ID names a guard in a profile and in the window.
func (g guard) ID() string { return g.Kind + ":" + g.Key }

// guards are the flags the window lists, variables first (see init).
// Every setting and variable here is one Claude Code 2.1.283 reads.
var guards = []guard{
	{Kind: "env", Key: "CCR_FORCE_BUNDLE", Known: true, On: "1",
		Desc: "Force claude --cloud to bundle and upload your local repository instead of cloning from its remote"},
	{Kind: "env", Key: "CLAUDE_AGENT_SDK_DISABLE_BUILTIN_AGENTS", Known: true, On: "1",
		Desc: "Disable all built-in subagent types such as Explore and Plan"},
	{Kind: "env", Key: "CLAUDE_AGENT_SDK_MCP_NO_PREFIX", Known: true, On: "1",
		Desc: "Skip the mcp__<server>__ prefix on tool names from SDK-created MCP servers"},
	{Kind: "env", Key: "CLAUDE_AUTO_BACKGROUND_TASKS", Known: true, On: "1",
		Desc: "Force-enable automatic backgrounding of long-running agent tasks"},
	{Kind: "env", Key: "CLAUDE_AX_SCREEN_READER", Known: true, On: "1",
		Desc: "Render screen-reader friendly output: flat text without decorative borders or animations"},
	{Kind: "env", Key: "CLAUDE_CODE_ACCESSIBILITY", Known: true, On: "1",
		Desc: "Keep the native terminal cursor visible and disable the inverted-text cursor indicator"},
	{Kind: "env", Key: "CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD", Known: true, On: "1",
		Desc: "Load memory files from directories specified with --add-dir"},
	{Kind: "env", Key: "CLAUDE_CODE_ALT_SCREEN_FULL_REPAINT", Known: true, On: "1",
		Desc: "Repaint the entire screen on every frame in fullscreen rendering instead of sending incremental updates"},
	{Kind: "env", Key: "CLAUDE_CODE_ALWAYS_ENABLE_EFFORT", Known: true, On: "1",
		Desc: "Send the effort parameter with every request, even when Claude Code does not recognize the model ID as effort-capable"},
	{Kind: "env", Key: "CLAUDE_CODE_ARTIFACT_AUTO_OPEN", On: "1", Off: "0",
		Desc: "Opens the browser automatically when a new artifact is published"},
	{Kind: "env", Key: "CLAUDE_CODE_ARTIFACT_COMMENTS", On: "1", Off: "0",
		Desc: "Claude reads and replies to comments on an artifact"},
	{Kind: "env", Key: "CLAUDE_CODE_ARTIFACT_COMMENTS_AUTOREACT", On: "1", Off: "0",
		Desc: "Claude replies on its own to comments sent to it"},
	{Kind: "env", Key: "CLAUDE_CODE_ATTRIBUTION_HEADER", On: "1", Off: "0",
		Desc: "Puts the attribution block, with the client version and a prompt fingerprint, at the start of the system prompt"},
	{Kind: "env", Key: "CLAUDE_CODE_AUTO_CONNECT_IDE", Ticked: true, Known: true, Default: true, On: "true", Off: "false",
		Desc: "Override automatic IDE connection"},
	{Kind: "env", Key: "CLAUDE_CODE_AUTO_MODE_SERVER", Ticked: true, Known: true, Default: true, On: "1", Off: "0",
		Desc: "Controls whether Claude Code asks the server to review auto mode actions"},
	{Kind: "env", Key: "CLAUDE_CODE_BG_TASKS_REPORT_RUNNING", Ticked: true, Known: true, Default: true, On: "1", Off: "0",
		Desc: "A non-interactive session reports a running status to its host while background work is still running"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_1M_CONTEXT", Known: true, On: "1",
		Desc: "Disable 1M context window support"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_ADAPTIVE_THINKING", Known: true, On: "1",
		Desc: "Disable adaptive reasoning on Opus 4.6 and Sonnet 4.6 and fall back to the fixed thinking budget controlled by MAX_THINKING_TOKENS"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_ADMIN_ENV_UNION", Known: true, On: "1",
		Desc: "Stop Claude Code from merging managed settings env blocks per key across admin sources, so only the highest-priority source's whole env block applies, as before v2.1.223"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_ADVISOR_TOOL", Known: true, On: "1",
		Desc: "Disable the advisor tool"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_AGENT_VIEW", Known: true, On: "1",
		Desc: "Turn off background agents and agent view: claude agents, --bg, /background, and the on-demand supervisor"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_ALTERNATE_SCREEN", Known: true, On: "1",
		Desc: "Disable fullscreen rendering and use the classic main-screen renderer"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_ARTIFACT", Ticked: true, On: "1",
		Desc: "Turn off the Artifact tool, which publishes session output as a private web page on claude.ai"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_ATTACHMENTS", Known: true, On: "1",
		Desc: "Disable attachment processing"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_AUTO_MEMORY", Known: true, On: "1",
		Desc: "Disable auto memory"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_BACKGROUND_TASKS", Known: true, On: "1",
		Desc: "Disable all background task functionality, including the run_in_background parameter on Bash and subagent tools, auto-backgrounding, and the Ctrl+B shortcut"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_BEDROCK_CONTENT_TYPE_DEFAULT", Known: true, On: "1",
		Desc: "Stop Claude Code from treating an Amazon Bedrock streaming response with a missing or empty Content-Type header as Amazon Bedrock's binary event stream"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_BEDROCK_CONTENT_TYPE_GUARD", Known: true, On: "1",
		Desc: "Skip the check that an Amazon Bedrock streaming response carries the application/vnd.amazon.eventstream content-type"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_BG_EXIT_HANDOFF", Known: true, On: "1",
		Desc: "Stop a background session's running background shell commands, dynamic workflows, and, as of v2.1.198, background subagents when the supervisor stops, restarts, or updates that session's…"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_BG_SHELL_PRESSURE_REAP", Known: true, On: "1",
		Desc: "Stop Claude Code from terminating background shell commands under memory pressure"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_BUNDLED_SKILLS", Known: true, On: "1",
		Desc: "Disable the skills and workflows included with Claude Code: bundled skills and workflows are removed entirely, while built-in commands like /init stay typable but are hidden from the model"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_CFC_PROMPT", Known: true, On: "1",
		Desc: "Keep the Claude in Chrome browser tools available while omitting the Chrome section of the system prompt and the /claude-in-chrome bundled skill"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_CLAUDE_MDS", Known: true, On: "1",
		Desc: "Prevent loading any CLAUDE.md memory files into context, including user, project, and auto memory files"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_CRON", Known: true, On: "1",
		Desc: "Disable scheduled tasks"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_DANGEROUS_RM_TIMEOUT", Known: true, On: "1",
		Desc: "Turn off the time limit on critical-path removal prompts"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS", Known: true, On: "1",
		Desc: "Strip Anthropic-specific anthropic-beta request headers and beta tool-schema fields (such as defer_loading and eager_input_streaming) from API requests"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_EXPLORE_PLAN_AGENTS", Known: true, On: "1",
		Desc: "Disable the built-in Explore and Plan subagents"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_FAST_MODE", Known: true, On: "1",
		Desc: "Disable fast mode"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_FEEDBACK_SURVEY", Ticked: true, On: "1",
		Desc: "Disable the \"How is Claude doing?\" session quality surveys"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_FILE_CHECKPOINTING", Known: true, On: "1",
		Desc: "Disable file checkpointing"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_GIT_INSTRUCTIONS", Known: true, On: "1",
		Desc: "Remove built-in commit and PR workflow instructions and the git status snapshot from Claude's context"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_LEGACY_MODEL_REMAP", Known: true, On: "1",
		Desc: "Prevent automatic remapping of Opus 4.0 and 4.1 to the current Opus version on the Anthropic API"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_MOUSE", Known: true, On: "1",
		Desc: "Disable mouse tracking in fullscreen rendering"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_MOUSE_CLICKS", Known: true, On: "1",
		Desc: "Disable click, drag, and hover handling in fullscreen rendering while keeping mouse-wheel scrolling"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_MTLS_RELOAD_ON_STALE_CONNECTION", Known: true, On: "1",
		Desc: "Stop Claude Code from re-reading the mTLS client certificate and key when an API request fails with a connection-level error, such as a connection reset or a TLS handshake error"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC", Ticked: true, On: "1",
		Desc: "Disable nonessential network traffic: auto-updates, telemetry, error reporting, the /feedback command, Claude-drafted feedback, release notes, the PR and MR…"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_NONSTREAMING_FALLBACK", Known: true, On: "1",
		Desc: "Disable the non-streaming fallback when a streaming request fails mid-stream"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_NOTIFICATION_PRESENCE_CHECK", Known: true, On: "1",
		Desc: "Send the PushNotification tool's desktop notification even while you are typing in or focused on the terminal"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_OFFICIAL_MARKETPLACE_AUTOINSTALL", Known: true, On: "1",
		Desc: "Disable automatic registration of the official plugin marketplace"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_PERMISSION_PROMPT_NOTIFY_HOOKS", Known: true, On: "1",
		Desc: "Stop Claude Code from running your Notification hooks for unanswered permission requests in sessions where Claude Code sends them to the Agent SDK's canUseTool callback, which is how…"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_POLICY_SKILLS", Known: true, On: "1",
		Desc: "Skip loading skills from the system-wide managed skills directory"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_POWERSHELL_CMD_RM_DENY", Known: true, On: "1",
		Desc: "Turn off the PowerShell tool check that denies the cmd built-ins rd, rmdir, del, and erase on a system path, such as a drive root or your home directory"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_SUBSTITUTION_RM_PROMPT", Known: true, On: "1",
		Desc: "Turn off the critical-path check for a recursive rm whose target is entirely the output of a command substitution, such as rm -rf \"$(pwd)\""},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_TERMINAL_TITLE", Known: true, On: "1",
		Desc: "Disable automatic terminal title updates based on conversation context"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_THINKING", Known: true, On: "1",
		Desc: "Omit the thinking parameter from API requests entirely"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_UNKNOWN_MODEL_WINDOW_ENFORCEMENT", Known: true, On: "1",
		Desc: "Skip proactive auto-compaction when Claude Code doesn't recognize the model ID, such as an LLM gateway alias"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_VIRTUAL_SCROLL", Known: true, On: "1",
		Desc: "Disable virtual scrolling in fullscreen rendering and render every message in the transcript"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_WINDOWS_SHELL_LAUNCHER", Known: true, On: "1",
		Desc: "Start PowerShell tool commands on Windows directly instead of through the cmd.exe launcher"},
	{Kind: "env", Key: "CLAUDE_CODE_DISABLE_WORKFLOWS", Known: true, On: "1",
		Desc: "Disable workflows"},
	{Kind: "env", Key: "CLAUDE_CODE_ENABLE_AWAY_SUMMARY", Ticked: true, Known: true, Default: true, On: "1", Off: "0",
		Desc: "Override session recap availability"},
	{Kind: "env", Key: "CLAUDE_CODE_ENABLE_BACKGROUND_PLUGIN_REFRESH", Known: true, On: "1",
		Desc: "Refresh plugin state at turn boundaries in non-interactive mode after a background install completes"},
	{Kind: "env", Key: "CLAUDE_CODE_ENABLE_FEEDBACK_SURVEY_FOR_OTEL", On: "1", Off: "0",
		Desc: "Route the \"How is Claude doing?\" session quality survey to your own OpenTelemetry collector when Anthropic-bound nonessential traffic is blocked"},
	{Kind: "env", Key: "CLAUDE_CODE_ENABLE_FINE_GRAINED_TOOL_STREAMING", Ticked: true, Known: true, Default: true, On: "1", Off: "0",
		Desc: "Controls whether tool call inputs stream from the API as Claude generates them"},
	{Kind: "env", Key: "CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY", Known: true, On: "1",
		Desc: "Populate the /model picker from your gateway's /v1/models endpoint when ANTHROPIC_BASE_URL points at an Anthropic-compatible gateway such as LiteLLM, Kong, or an internal proxy"},
	{Kind: "env", Key: "CLAUDE_CODE_ENABLE_PROMPT_SUGGESTION", Ticked: true, Known: true, Default: true, On: "true", Off: "false",
		Desc: "Prompt suggestions, the grayed-out predictions in the prompt input"},
	{Kind: "env", Key: "CLAUDE_CODE_ENABLE_TASKS", Ticked: true, Known: true, Default: true, On: "1", Off: "0",
		Desc: "Selects which task-tracking tools Claude Code provides in sessions that have them"},
	{Kind: "env", Key: "CLAUDE_CODE_ENABLE_TELEMETRY", On: "1", Off: "0",
		Desc: "Enable OpenTelemetry data collection for metrics and logging"},
	{Kind: "env", Key: "CLAUDE_CODE_ENABLE_TODO_TOOLS", Known: true, On: "1",
		Desc: "Get the task-tracking tools on every model"},
	{Kind: "env", Key: "CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS", Known: true, On: "1",
		Desc: "Enable agent teams"},
	{Kind: "env", Key: "CLAUDE_CODE_FORCE_SESSION_PERSISTENCE", Known: true, On: "1",
		Desc: "Force transcript persistence, prompt history, and claude agents registration even when this claude was launched from inside another Claude Code session"},
	{Kind: "env", Key: "CLAUDE_CODE_FORCE_STRIKETHROUGH", Known: true, On: "1",
		Desc: "Force strikethrough rendering for ~~text~~ in Claude's responses when your terminal supports it but is not auto-detected, such as over SSH without TERM_PROGRAM forwarded"},
	{Kind: "env", Key: "CLAUDE_CODE_FORWARD_SUBAGENT_TEXT", Known: true, On: "1",
		Desc: "Emit subagent text and thinking blocks in claude -p --output-format stream-json output, the same behavior as the --forward-subagent-text flag"},
	{Kind: "env", Key: "CLAUDE_CODE_GATEWAY_HINT_HEADERS", Known: true, On: "1",
		Desc: "Send the gateway hint headers, such as x-claude-code-request-class and x-claude-code-compaction, on a custom proxy or a third-party provider such as Amazon Bedrock or Claude Platform on…"},
	{Kind: "env", Key: "CLAUDE_CODE_GLOB_HIDDEN", Ticked: true, Known: true, Default: true, On: "true", Off: "false",
		Desc: "The Glob tool includes dotfiles in its results"},
	{Kind: "env", Key: "CLAUDE_CODE_GLOB_NO_IGNORE", Ticked: true, Known: true, Default: true, On: "true", Off: "false",
		Desc: "The Glob tool ignores .gitignore patterns"},
	{Kind: "env", Key: "CLAUDE_CODE_HIDE_CWD", Known: true, On: "1",
		Desc: "Hide the working directory in the startup logo"},
	{Kind: "env", Key: "CLAUDE_CODE_IDE_SKIP_AUTO_INSTALL", Known: true, On: "1",
		Desc: "Skip auto-installation of IDE extensions"},
	{Kind: "env", Key: "CLAUDE_CODE_IDE_SKIP_VALID_CHECK", Known: true, On: "1",
		Desc: "Skip validation of IDE lockfile entries during connection"},
	{Kind: "env", Key: "CLAUDE_CODE_MCP_ALLOWLIST_ENV", Known: true, On: "1",
		Desc: "Spawn stdio MCP servers with only a safe baseline environment plus the server's configured env, instead of inheriting your shell environment"},
	{Kind: "env", Key: "CLAUDE_CODE_NATIVE_CURSOR", Known: true, On: "1",
		Desc: "Show the terminal's own cursor at the input caret instead of a drawn block"},
	{Kind: "env", Key: "CLAUDE_CODE_NEW_INIT", Known: true, On: "1",
		Desc: "Make /init run an interactive setup flow"},
	{Kind: "env", Key: "CLAUDE_CODE_NONBLOCKING_STDOUT", Known: true, On: "1",
		Desc: "Write terminal output through a second non-blocking file descriptor, so a terminal that stops reading, such as a paused tmux control-mode pane or a stalled SSH connection, can't freeze…"},
	{Kind: "env", Key: "CLAUDE_CODE_NO_FLICKER", Known: true, On: "1",
		Desc: "Enable fullscreen rendering, a research preview that reduces flicker and keeps memory flat in long conversations"},
	{Kind: "env", Key: "CLAUDE_CODE_OTEL_DIAG_STDERR", Known: true, On: "1",
		Desc: "Write OpenTelemetry exporter diagnostic errors to stderr"},
	{Kind: "env", Key: "CLAUDE_CODE_PACKAGE_MANAGER_AUTO_UPDATE", Known: true, On: "1",
		Desc: "Let Claude Code run your package manager's upgrade command in the background when a new version is available"},
	{Kind: "env", Key: "CLAUDE_CODE_PERFORCE_MODE", Known: true, On: "1",
		Desc: "Enable Perforce-aware write protection"},
	{Kind: "env", Key: "CLAUDE_CODE_PLUGIN_KEEP_MARKETPLACE_ON_FAILURE", Known: true, On: "1",
		Desc: "Skip the re-clone attempt and keep using the existing marketplace checkout when a marketplace refresh can't reach or authenticate to the remote"},
	{Kind: "env", Key: "CLAUDE_CODE_PLUGIN_PREFER_HTTPS", Known: true, On: "1",
		Desc: "Clone GitHub owner/repo shorthand sources over HTTPS instead of SSH"},
	{Kind: "env", Key: "CLAUDE_CODE_POWERSHELL_RESPECT_EXECUTION_POLICY", Known: true, On: "1",
		Desc: "Stop Claude Code from passing -ExecutionPolicy Bypass when spawning PowerShell for tool calls, hooks, and status line commands, and respect the machine's effective execution policy instead"},
	{Kind: "env", Key: "CLAUDE_CODE_PROPAGATE_TRACEPARENT", Known: true, On: "1",
		Desc: "Propagate W3C trace context when ANTHROPIC_BASE_URL points at a custom proxy"},
	{Kind: "env", Key: "CLAUDE_CODE_PROXY_RESOLVES_HOSTS", Known: true, On: "1",
		Desc: "Allow the proxy to perform DNS resolution instead of the caller"},
	{Kind: "env", Key: "CLAUDE_CODE_RESTRICTED", Known: true, On: "1",
		Desc: "Start the session in restricted mode, the same as passing --restricted"},
	{Kind: "env", Key: "CLAUDE_CODE_RESUME_INTERRUPTED_TURN", Known: true, On: "1",
		Desc: "Automatically resume if the previous session ended mid-turn"},
	{Kind: "env", Key: "CLAUDE_CODE_RETRY_WATCHDOG", Known: true, On: "1",
		Desc: "Retries 429 and 529 capacity errors indefinitely, for unattended sessions such as CI jobs"},
	{Kind: "env", Key: "CLAUDE_CODE_SAFE_MODE", Known: true, On: "1",
		Desc: "Start in safe mode: CLAUDE.md, skills, plugins, hooks, MCP servers, custom commands and agents, output styles, workflows, custom themes, custom keybindings, status line and…"},
	{Kind: "env", Key: "CLAUDE_CODE_SEND_FEEDBACK", On: "1", Off: "0",
		Desc: "Claude-drafted feedback"},
	{Kind: "env", Key: "CLAUDE_CODE_SIMPLE", Known: true, On: "1",
		Desc: "Run with a minimal system prompt and only the Bash, file read, and file edit tools"},
	{Kind: "env", Key: "CLAUDE_CODE_SIMPLE_SYSTEM_PROMPT", Known: true, On: "1",
		Desc: "Use a shorter system prompt and abbreviated tool descriptions on any model"},
	{Kind: "env", Key: "CLAUDE_CODE_SKIP_AWS_CRED_CACHE", Known: true, On: "1",
		Desc: "Turn off the in-process cache of credentials resolved from the AWS default credential provider chain, so Claude Code resolves the chain on every API request"},
	{Kind: "env", Key: "CLAUDE_CODE_SKIP_FAST_MODE_NETWORK_ERRORS", Known: true, On: "1",
		Desc: "Treat a failed fast mode availability check as available, for networks that block the check's direct request to api.anthropic.com"},
	{Kind: "env", Key: "CLAUDE_CODE_SKIP_FAST_MODE_ORG_CHECK", Known: true, On: "1",
		Desc: "Skip the client-side fast mode availability check, for proxies that intercept the check's request rather than refuse it"},
	{Kind: "env", Key: "CLAUDE_CODE_SKIP_PROMPT_HISTORY", Known: true, On: "1",
		Desc: "Skip writing prompt history and session transcripts to disk"},
	{Kind: "env", Key: "CLAUDE_CODE_STARTUP_FAILURE_RESULTS", Known: true, On: "1",
		Desc: "Have a session started with --output-format stream-json write a result message naming why Claude Code refused to start for startup failures that otherwise end with stderr alone"},
	{Kind: "env", Key: "CLAUDE_CODE_SUBAGENT_MODEL_FORCE", Known: true, On: "1",
		Desc: "Force one model onto subagents, teammates, and workflow agents"},
	{Kind: "env", Key: "CLAUDE_CODE_SUBPROCESS_ENV_SCRUB", Known: true, On: "1",
		Desc: "Strip credentials from subprocess environments (Bash tool, hooks, MCP stdio servers): Anthropic and cloud provider credentials, any other variable that Claude Code recognizes as a…"},
	{Kind: "env", Key: "CLAUDE_CODE_SUPPRESS_SESSION_ATTRIBUTION", Ticked: true, On: "1",
		Desc: "Leave session links out of pull requests"},
	{Kind: "env", Key: "CLAUDE_CODE_SYNC_PLUGIN_INSTALL", Known: true, On: "1",
		Desc: "Complete before the first query"},
	{Kind: "env", Key: "CLAUDE_CODE_SYNC_SKILLS", Known: true, On: "1",
		Desc: "Make Claude Code download the skills enabled for your claude.ai account in that run and wait for the list of them, up to…"},
	{Kind: "env", Key: "CLAUDE_CODE_SYNTAX_HIGHLIGHT", Ticked: true, Known: true, Default: true, On: "true", Off: "false",
		Desc: "Syntax highlighting in diff output"},
	{Kind: "env", Key: "CLAUDE_CODE_TMUX_TRUECOLOR", Known: true, On: "1",
		Desc: "Allow 24-bit truecolor output inside tmux"},
	{Kind: "env", Key: "CLAUDE_CODE_USE_NATIVE_FILE_SEARCH", Known: true, On: "1",
		Desc: "Discover custom commands, subagents, and output styles using Node.js file APIs instead of ripgrep"},
	{Kind: "env", Key: "CLAUDE_DISABLE_ADOPT", Known: true, On: "1",
		Desc: "Stop in-flight background work instead of carrying it over when you background a session by pressing ← or with /background"},
	{Kind: "env", Key: "CLAUDE_ENABLE_BYTE_WATCHDOG_BEDROCK", Known: true, On: "1",
		Desc: "Enable the byte-level streaming idle watchdog on Amazon Bedrock vnd.amazon.eventstream responses, which also enables the first-byte deadline on Bedrock streaming requests"},
	{Kind: "env", Key: "DEBUG", Known: true, On: "1",
		Desc: "Enable debug mode, equivalent to launching with --debug"},
	{Kind: "env", Key: "DISABLE_AUTOUPDATER", Known: true, On: "1",
		Desc: "Disable automatic background updates"},
	{Kind: "env", Key: "DISABLE_AUTO_COMPACT", Known: true, On: "1",
		Desc: "Disable automatic compaction when approaching the context limit"},
	{Kind: "env", Key: "DISABLE_BUG_COMMAND", Ticked: true, On: "1",
		Desc: "Turn off /bug, which sends the session to Anthropic"},
	{Kind: "env", Key: "DISABLE_COMPACT", Known: true, On: "1",
		Desc: "Disable all compaction: both automatic compaction and the manual /compact command"},
	{Kind: "env", Key: "DISABLE_COST_WARNINGS", Known: true, On: "1",
		Desc: "Disable cost warning messages"},
	{Kind: "env", Key: "DISABLE_DOCTOR_COMMAND", Known: true, On: "1",
		Desc: "Hide the /doctor setup checkup skill and its /checkup alias"},
	{Kind: "env", Key: "DISABLE_ERROR_REPORTING", Ticked: true, On: "1",
		Desc: "Opt out of error reporting"},
	{Kind: "env", Key: "DISABLE_EXTRA_USAGE_COMMAND", Known: true, On: "1",
		Desc: "Hide the /usage-credits command that lets users purchase additional usage beyond rate limits"},
	{Kind: "env", Key: "DISABLE_FEEDBACK_COMMAND", Ticked: true, On: "1",
		Desc: "Disable the /feedback command and Claude-drafted feedback"},
	{Kind: "env", Key: "DISABLE_GROWTHBOOK", Ticked: true, On: "1",
		Desc: "Turns off GrowthBook feature-flag fetching and uses code defaults for every flag"},
	{Kind: "env", Key: "DISABLE_INSTALLATION_CHECKS", Known: true, On: "1",
		Desc: "Disable installation warnings"},
	{Kind: "env", Key: "DISABLE_INSTALL_GITHUB_APP_COMMAND", Known: true, On: "1",
		Desc: "Hide the /install-github-app command"},
	{Kind: "env", Key: "DISABLE_INTERLEAVED_THINKING", Known: true, On: "1",
		Desc: "Prevent sending the interleaved-thinking beta header"},
	{Kind: "env", Key: "DISABLE_LOGIN_COMMAND", Known: true, On: "1",
		Desc: "Hide the /login command"},
	{Kind: "env", Key: "DISABLE_LOGOUT_COMMAND", Known: true, On: "1",
		Desc: "Hide the /logout command"},
	{Kind: "env", Key: "DISABLE_PROMPT_CACHING", Known: true, On: "1",
		Desc: "Disable prompt caching for all models (takes precedence over per-model settings)"},
	{Kind: "env", Key: "DISABLE_PROMPT_CACHING_FABLE", Known: true, On: "1",
		Desc: "Disable prompt caching for Fable models"},
	{Kind: "env", Key: "DISABLE_PROMPT_CACHING_HAIKU", Known: true, On: "1",
		Desc: "Disable prompt caching for the default Haiku model, wherever it runs"},
	{Kind: "env", Key: "DISABLE_PROMPT_CACHING_OPUS", Known: true, On: "1",
		Desc: "Disable prompt caching for Opus models"},
	{Kind: "env", Key: "DISABLE_PROMPT_CACHING_SONNET", Known: true, On: "1",
		Desc: "Disable prompt caching for Sonnet models"},
	{Kind: "env", Key: "DISABLE_TELEMETRY", Ticked: true, On: "1",
		Desc: "Opt out of telemetry"},
	{Kind: "env", Key: "DISABLE_UPDATES", Known: true, On: "1",
		Desc: "Block all updates including manual claude update and claude install"},
	{Kind: "env", Key: "DISABLE_UPGRADE_COMMAND", Known: true, On: "1",
		Desc: "Hide the /upgrade command"},
	{Kind: "env", Key: "DO_NOT_TRACK", Ticked: true, On: "1",
		Desc: "Opt out of telemetry, with the same effect as DISABLE_TELEMETRY, including on feature-flag fetching"},
	{Kind: "deny", Key: "DesignSync", Known: true, Default: true,
		Desc: "Lets Claude sync with Claude Design on claude.ai with the DesignSync tool"},
	{Kind: "env", Key: "ENABLE_BETA_TRACING_DETAILED", On: "1", Off: "0",
		Desc: "Detailed beta tracing, which adds content-bearing span attributes and the claude_code.hook span"},
	{Kind: "env", Key: "ENABLE_CLAUDEAI_MCP_SERVERS", On: "true", Off: "false",
		Desc: "Claude Code fetches MCP servers (connectors) from claude.ai"},
	{Kind: "env", Key: "ENABLE_PROMPT_CACHING_1H", Known: true, On: "1",
		Desc: "Request a 1-hour prompt cache TTL instead of the default 5 minutes"},
	{Kind: "env", Key: "FALLBACK_FOR_ALL_PRIMARY_MODELS", Known: true, On: "1",
		Desc: "Make Claude Code stop retrying on repeated overload errors for every model when no fallback model is configured"},
	{Kind: "env", Key: "FORCE_AUTOUPDATE_PLUGINS", Known: true, On: "1",
		Desc: "Force plugin auto-updates even when the main auto-updater is disabled via DISABLE_AUTOUPDATER"},
	{Kind: "env", Key: "FORCE_PROMPT_CACHING_5M", Known: true, On: "1",
		Desc: "Force the 5-minute prompt cache TTL even when 1-hour TTL would otherwise apply"},
	{Kind: "env", Key: "IS_DEMO", Known: true, On: "1",
		Desc: "Enable demo mode: hides your email and organization name from the header and /status output, and skips onboarding"},
	{Kind: "env", Key: "MCP_CONNECTION_NONBLOCKING", Ticked: true, Known: true, Default: true, On: "1", Off: "0",
		Desc: "Controls whether startup waits for MCP servers to connect before the first query"},
	{Kind: "env", Key: "OTEL_LOG_ASSISTANT_RESPONSES", On: "1", Off: "0",
		Desc: "Include the model's response text on assistant_response OpenTelemetry log events"},
	{Kind: "env", Key: "OTEL_LOG_MANAGED_SETTINGS", Known: true, On: "1",
		Desc: "Add the redacted managed settings, and a SHA-256 digest of the settings before redaction, to managed_settings_resolved OpenTelemetry log events"},
	{Kind: "env", Key: "OTEL_LOG_RAW_API_BODIES", On: "1", Off: "0",
		Desc: "Emit Anthropic Messages API request and response JSON as api_request_body / api_response_body log events"},
	{Kind: "env", Key: "OTEL_LOG_TOOL_CONTENT", On: "1", Off: "0",
		Desc: "Include tool content in the tool.output OpenTelemetry span event"},
	{Kind: "env", Key: "OTEL_LOG_TOOL_DETAILS", On: "1", Off: "0",
		Desc: "Include tool input arguments, MCP server names, user-authored workflow names, raw error strings on tool failures, the refusal category on api_refusal events, and other tool details in…"},
	{Kind: "env", Key: "OTEL_LOG_USER_PROMPTS", On: "1", Off: "0",
		Desc: "Include user prompt text in OpenTelemetry traces and logs"},
	{Kind: "env", Key: "OTEL_METRICS_INCLUDE_ACCOUNT_UUID", Ticked: true, Known: true, Default: true, On: "true", Off: "false",
		Desc: "Includes the account UUID in metrics attributes"},
	{Kind: "env", Key: "OTEL_METRICS_INCLUDE_ENTRYPOINT", Known: true, On: "true",
		Desc: "Include the session entrypoint in metrics attributes (default: excluded)"},
	{Kind: "env", Key: "OTEL_METRICS_INCLUDE_REPOSITORY", On: "true", Off: "false",
		Desc: "Tag OpenTelemetry metrics and events with vcs.* attributes identifying the session's repository (default: excluded)"},
	{Kind: "env", Key: "OTEL_METRICS_INCLUDE_RESOURCE_ATTRIBUTES", Ticked: true, Known: true, Default: true, On: "true", Off: "false",
		Desc: "As of v2.1.161, Claude Code attaches OTEL_RESOURCE_ATTRIBUTES keys to metric datapoint labels"},
	{Kind: "env", Key: "OTEL_METRICS_INCLUDE_SESSION_ID", Ticked: true, Known: true, Default: true, On: "true", Off: "false",
		Desc: "Includes the session ID in metrics attributes"},
	{Kind: "env", Key: "OTEL_METRICS_INCLUDE_VERSION", Known: true, On: "true",
		Desc: "Include Claude Code version in metrics attributes (default: excluded)"},
	{Kind: "deny", Key: "PushNotification", Known: true, Default: true,
		Desc: "Lets Claude send notifications to the phone with the PushNotification tool"},
	{Kind: "deny", Key: "RemoteTrigger", Known: true, Default: true,
		Desc: "Lets Claude run cloud routines with the RemoteTrigger tool"},
	{Kind: "env", Key: "USE_BUILTIN_RIPGREP", Ticked: true, Known: true, Default: true, On: "1", Off: "0",
		Desc: "Uses the rg included with Claude Code instead of the system-installed one"},
	{Kind: "setting", Key: "agentPushNotifEnabled", On: true, Off: false,
		Desc: "Allow Claude to send a push notification to your phone when it decides one is worth sending, for example when a long task finishes"},
	{Kind: "setting", Key: "attribution.commit", Off: "",
		Desc: "Claude attribution in commit messages"},
	{Kind: "setting", Key: "attribution.pr", Off: "",
		Desc: "Claude attribution in pull request descriptions"},
	{Kind: "setting", Key: "autoContinueAtUsageLimit", Ticked: true, Known: true, Default: true, On: true, Off: false,
		Desc: "After a claude.ai usage limit stops your session, wait in the open session and continue the task automatically after the reset"},
	{Kind: "setting", Key: "autoMode.classifyAllShell", Known: true, On: true, Off: false,
		Desc: "Send every Bash and PowerShell command through the auto mode classifier while auto mode is active"},
	{Kind: "setting", Key: "autoScrollEnabled", Ticked: true, Known: true, Default: true, On: true, Off: false,
		Desc: "Follow new output to the bottom of the conversation in fullscreen rendering"},
	{Kind: "setting", Key: "autoUploadSessions", On: true, Off: false,
		Desc: "Mirror local sessions to claude.ai as view-only"},
	{Kind: "setting", Key: "bashEditDiffEnabled", Known: true, On: true, Off: false,
		Desc: "Choose whether Claude Code records which files changed in a Git repository while a Bash command runs"},
	{Kind: "setting", Key: "disableAllHooks", Known: true, On: true, Off: false,
		Desc: "Turn off hooks, any custom status line, and any custom file suggestion command"},
	{Kind: "setting", Key: "disableAutoMode", Known: true, On: "disable",
		Desc: "Remove auto mode from the Shift+Tab cycle"},
	{Kind: "setting", Key: "disableDeepLinkRegistration", Known: true, On: "disable",
		Desc: "Stop Claude Code from registering the claude-cli:// protocol handler with the operating system, which it otherwise does after you send the first prompt of an interactive session"},
	{Kind: "setting", Key: "disableRemoteControl", Ticked: true, On: true, Off: false,
		Desc: "Turn off Remote Control: Claude Code then refuses claude remote-control, the --remote-control flag, auto-start, and the in-session toggle, and reports that your organization's policy disabled it"},
	{Kind: "setting", Key: "disableSkillShellExecution", Known: true, On: true, Off: false,
		Desc: "Turn off inline shell execution for  !"},
	{Kind: "setting", Key: "emojiCompletionEnabled", Ticked: true, Known: true, Default: true, On: true, Off: false,
		Desc: "Show emoji suggestions when you type : plus a shortcode in the prompt input, and replace a completed shortcode such as :heart: with its emoji"},
	{Kind: "setting", Key: "enableAllProjectMcpServers", Known: true, On: true, Off: false,
		Desc: "Approve every MCP server defined in project .mcp.json files without a prompt"},
	{Kind: "setting", Key: "enforceAvailableModels", Known: true, On: true, Off: false,
		Desc: "The /model picker has a Default option that resolves to your organization default model when one applies, and otherwise to your account type's default"},
	{Kind: "setting", Key: "fastModePerSessionOptIn", Known: true, On: true, Off: false,
		Desc: "Normally, running /fast saves fastMode to a person's user settings, so fast mode is on at the start of every later session"},
	{Kind: "setting", Key: "includeCoAuthoredBy", On: true, Off: false,
		Desc: "<Warning>\n  Deprecated since v2.0.62, when attribution replaced it"},
	{Kind: "setting", Key: "inputNeededNotifEnabled", On: true, Off: false,
		Desc: "Get a push notification on your phone when a permission prompt or question is waiting for your input"},
	{Kind: "setting", Key: "isolatePeerMachines", Ticked: true, On: true, Off: false,
		Desc: "Require your explicit approval before Claude's SendMessage reaches one of your sessions beyond this machine"},
	{Kind: "setting", Key: "permissions.blockReadsOutsideWorkingDirectories", Known: true, On: true, Off: false,
		Desc: "Stop Claude from reading paths outside the session's working directories with the Read, Grep, Glob, and LSP tools, in every permission mode including bypassPermissions"},
	{Kind: "setting", Key: "permissions.disableBypassPermissionsMode", Known: true, On: "disable",
		Desc: "Prevent anyone from entering bypassPermissions mode"},
	{Kind: "setting", Key: "prefersReducedMotion", Known: true, On: true, Off: false,
		Desc: "Reduce or turn off interface animations such as the spinner, shimmer, and flash effects"},
	{Kind: "setting", Key: "remoteControlAtStartup", On: true, Off: false,
		Desc: "Connect Remote Control automatically when each interactive session starts, instead of waiting for /remote-control"},
	{Kind: "setting", Key: "respectGitignore", Ticked: true, Known: true, Default: true, On: true, Off: false,
		Desc: "Control whether the @ file picker leaves out files that match .gitignore patterns"},
	{Kind: "setting", Key: "respondToBashCommands", Ticked: true, Known: true, Default: true, On: true, Off: false,
		Desc: "Choose whether Claude responds after you run a shell command with the ! prefix in the input box"},
	{Kind: "setting", Key: "sandbox.allowAppleEvents", Known: true, On: true, Off: false,
		Desc: "Let sandboxed commands on macOS send Apple Events, which open, osascript, and tools that open URLs in a browser need"},
	{Kind: "setting", Key: "sandbox.allowUnsandboxedCommands", Ticked: true, Known: true, Default: true, On: true, Off: false,
		Desc: "Let Claude retry a command outside the sandbox with the dangerouslyDisableSandbox parameter after the sandbox blocks it"},
	{Kind: "setting", Key: "sandbox.autoAllowBashIfSandboxed", Ticked: true, Known: true, Default: true, On: true, Off: false,
		Desc: "Let Claude Code run sandboxed Bash commands without a permission prompt"},
	{Kind: "setting", Key: "sandbox.credentials.allowPlaintextInject", Known: true, On: true, Off: false,
		Desc: "Allow mask substitution on plain HTTP requests as well as TLS-terminated HTTPS"},
	{Kind: "setting", Key: "sandbox.enableWeakerNestedSandbox", Known: true, On: true, Off: false,
		Desc: "Run the Linux sandbox inside an unprivileged Docker container, where bubblewrap can't mount a fresh /proc"},
	{Kind: "setting", Key: "sandbox.enableWeakerNetworkIsolation", Known: true, On: true, Off: false,
		Desc: "Let sandboxed commands on macOS reach the system TLS trust service, com.apple.trustd.agent"},
	{Kind: "setting", Key: "sandbox.enabled", Known: true, On: true, Off: false,
		Desc: "Turn on sandboxing for Bash commands"},
	{Kind: "setting", Key: "sandbox.failIfUnavailable", Known: true, On: true, Off: false,
		Desc: "Make Claude Code exit with an error at startup when sandbox.enabled is true but the sandbox can't start, because a dependency is missing or the platform is unsupported"},
	{Kind: "setting", Key: "sandbox.filesystem.disabled", Known: true, On: true, Off: false,
		Desc: "Skip filesystem isolation while keeping network isolation"},
	{Kind: "setting", Key: "sandbox.network.allowAllUnixSockets", Known: true, On: true, Off: false,
		Desc: "Let sandboxed commands connect to every Unix socket"},
	{Kind: "setting", Key: "sandbox.network.allowLocalBinding", Known: true, On: true, Off: false,
		Desc: "Let sandboxed commands bind to localhost ports on macOS, for example to start a dev server"},
	{Kind: "setting", Key: "sandbox.network.strictAllowlist", Known: true, On: true, Off: false,
		Desc: "Deny sandboxed commands access to hosts outside the allowlist instead of prompting for approval"},
	{Kind: "setting", Key: "showClearContextOnPlanAccept", Known: true, On: true, Off: false,
		Desc: "When Claude finishes a plan in plan mode, it shows an approval menu"},
	{Kind: "setting", Key: "showThinkingSummaries", Known: true, On: true, Off: false,
		Desc: "See summaries of Claude's extended thinking in interactive sessions"},
	{Kind: "setting", Key: "showTurnDuration", Ticked: true, Known: true, Default: true, On: true, Off: false,
		Desc: "Show or hide the turn duration message after each response, such as \"Cooked for 1m 6s · done 6:05 PM\""},
	{Kind: "setting", Key: "skipAutoPermissionPrompt", Known: true, On: true, Off: false,
		Desc: "Skip the one-time notice describing auto mode that Claude Code shows when you first enter auto mode yourself, for example through your own settings or the mode selector, rather than when the built-in…"},
	{Kind: "setting", Key: "skipDangerousModePermissionPrompt", Known: true, On: true, Off: false,
		Desc: "Skip the confirmation dialog Claude Code shows before a session enters bypassPermissions mode, whether from --dangerously-skip-permissions or from defaultMode: \"bypassPermissions\""},
	{Kind: "setting", Key: "skipWebFetchPreflight", Ticked: true, On: true, Off: false,
		Desc: "Skip the WebFetch domain safety check, which sends each requested hostname to api.anthropic.com before fetching"},
	{Kind: "setting", Key: "spinnerTipsEnabled", Ticked: true, Known: true, Default: true, On: true, Off: false,
		Desc: "While Claude works, the spinner line rotates through short tips about Claude Code features, such as \"Use Plan Mode to prepare for a complex request before making changes"},
	{Kind: "setting", Key: "switchModelsOnFlag", Ticked: true, Known: true, Default: true, On: true, Off: false,
		Desc: "Choose what happens when a safety classifier flags a request: switch to the fallback model and continue, or pause so you can choose between switching and editing the prompt"},
	{Kind: "setting", Key: "syncClaudeAiPlugins", On: true, Off: false,
		Desc: "Turn off the download of the plugins enabled for your claude.ai account"},
	{Kind: "setting", Key: "syncClaudeAiSkills", On: true, Off: false,
		Desc: "Turn off the download of the skills enabled for your claude.ai account"},
	{Kind: "setting", Key: "terminalProgressBarEnabled", Ticked: true, Known: true, Default: true, On: true, Off: false,
		Desc: "Some terminals can show a progress indicator on the tab or in the taskbar for the program running in them"},
	{Kind: "setting", Key: "terminalTitleFromRename", Ticked: true, Known: true, Default: true, On: true, Off: false,
		Desc: "Claude Code sets your terminal tab's title"},
	{Kind: "setting", Key: "ultracode", Known: true, On: true, Off: false,
		Desc: "Start sessions with ultracode on"},
	{Kind: "setting", Key: "useAutoModeDuringPlan", Ticked: true, Known: true, Default: true, On: true, Off: false,
		Desc: "Choose whether Claude Code uses the auto mode classifier to review shell commands in plan mode"},
	{Kind: "setting", Key: "verbose", Known: true, On: true, Off: false,
		Desc: "By default, the transcript collapses each tool call to a short summary, such as the command Claude ran and a line count of its output, and you press Ctrl+O to switch the whole transcript to the…"},
	{Kind: "setting", Key: "voiceEnabled", On: true, Off: false,
		Desc: "<Warning>\n  Deprecated since v2.1.92, when the voice object replaced it"},
	{Kind: "setting", Key: "wheelScrollAccelerationEnabled", Ticked: true, Known: true, Default: true, On: true, Off: false,
		Desc: "Accelerate mouse-wheel scroll speed during fast scrolls in fullscreen rendering"},
	{Kind: "setting", Key: "workflowKeywordTriggerEnabled", Ticked: true, Known: true, Default: true, On: true, Off: false,
		Desc: "Choose whether typing the keyword ultracode in a prompt triggers a dynamic workflow"},
	{Kind: "env", Key: "CLAUDE_CODE_BASH_EDIT_DIFF", Ticked: true, Known: true, Default: true, On: "1", Off: "0",
		Desc: "Set to 0 to turn off the diff of the files that changed while a Bash command ran, or 1 to record it in every permission mode"},
	{Kind: "env", Key: "CLAUDE_CODE_BS_AS_CTRL_BACKSPACE", Ticked: true, Known: true, Default: true, On: "1", Off: "0",
		Desc: "Set to 0 to make Claude Code read the 0x08 byte, also written ^H, as plain Backspace, or 1 to read it as Ctrl+Backspace"},
	{Kind: "env", Key: "CLAUDE_ENABLE_STREAM_WATCHDOG", Ticked: true, Known: true, Default: true, On: "1", Off: "0",
		Desc: "Set to 0 to force-disable the event-level streaming idle watchdog, or set to 1 to force-enable it"},
	{Kind: "env", Key: "CLAUDE_CODE_FORCE_SYNC_OUTPUT", Known: true, On: "1",
		Desc: "Force-enable DEC private mode 2026 synchronized output when your terminal supports it but is not auto-detected"},
	{Kind: "env", Key: "CLAUDE_CODE_FORK_SUBAGENT", Known: true, On: "1",
		Desc: "Controls fork mode, which lets Claude spawn forked subagents itself and is on by default in interactive sessions only"},
	{Kind: "env", Key: "CLAUDE_CODE_USE_POWERSHELL_TOOL", Ticked: true, Known: true, Default: true, On: "1", Off: "0",
		Desc: "Controls the PowerShell tool"},
	{Kind: "env", Key: "CLAUDE_ENABLE_BYTE_WATCHDOG", Known: true, On: "1",
		Desc: "Force-enable the byte-level streaming idle watchdog, or set to 0 to force-disable it"},
	{Kind: "env", Key: "FORCE_HYPERLINK", Known: true, On: "1",
		Desc: "Enable clickable OSC 8 hyperlinks when your terminal supports them but isn't auto-detected, or 0 to disable them"},
	{Kind: "env", Key: "MCP_DISCOVERY_CACHE", Known: true, On: "1",
		Desc: "Turns the MCP discovery cache on or off"},
}

// The window lists the variables first, the names all in capitals, in
// alphabetical order, then the settings and tools, whose names have small
// letters, in alphabetical order.
func init() {
	capitals := func(g guard) bool { return g.Key == strings.ToUpper(g.Key) }
	slices.SortStableFunc(guards, func(a, b guard) int {
		if ca, cb := capitals(a), capitals(b); ca != cb {
			if ca {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Key, b.Key)
	})
}

// ticked reports whether a guard's box is ticked for a profile: its own
// choice, or else where the box starts.
func (g guard) ticked(choices map[string]bool) bool {
	if v, ok := choices[g.ID()]; ok {
		return v
	}
	return g.Ticked
}

// value is what a box puts in the file, and whether it puts anything.
func (g guard) value(ticked bool) (any, bool) {
	v := g.Off
	if ticked {
		v = g.On
	}
	if v == nil || g.Known && g.Default == ticked {
		return nil, false
	}
	return v, true
}

// putGuards puts into a settings.json the flags as a profile has them.
func putGuards(m map[string]any, env map[string]any, choices map[string]bool) {
	var deny []any
	if perms, ok := m["permissions"].(map[string]any); ok {
		deny, _ = perms["deny"].([]any)
	}
	for _, g := range guards {
		t := g.ticked(choices)
		if g.Kind == "deny" {
			if !t && !slices.Contains(deny, any(g.Key)) {
				deny = append(deny, g.Key)
			}
			continue
		}
		v, ok := g.value(t)
		if !ok {
			continue
		}
		if g.Kind == "env" {
			env[g.Key] = v
		} else {
			setPath(m, g.Key, v)
		}
	}
	if len(deny) > 0 {
		perms, _ := m["permissions"].(map[string]any)
		if perms == nil {
			perms = map[string]any{}
		}
		perms["deny"] = deny
		m["permissions"] = perms
	}
}

// takeGuards takes every guard's setting, variable and denied tool out of a
// settings.json, and reports whether there were any. In a profile's
// settings.json these belong to the app.
func takeGuards(m map[string]any, env map[string]any) bool {
	changed := false
	perms, _ := m["permissions"].(map[string]any)
	deny, _ := perms["deny"].([]any)
	for _, g := range guards {
		switch g.Kind {
		case "setting":
			changed = dropPath(m, g.Key) || changed
		case "env":
			if _, ok := env[g.Key]; ok {
				delete(env, g.Key)
				changed = true
			}
		case "deny":
			if i := slices.Index(deny, any(g.Key)); i >= 0 {
				deny = slices.Delete(deny, i, i+1)
				changed = true
			}
		}
	}
	if perms != nil {
		if len(deny) == 0 {
			delete(perms, "deny")
		} else {
			perms["deny"] = deny
		}
		if len(perms) == 0 {
			delete(m, "permissions")
		}
	}
	return changed
}

// setPath sets a setting at a dotted path, making the objects on the way.
func setPath(m map[string]any, path string, v any) {
	parts := strings.Split(path, ".")
	for _, p := range parts[:len(parts)-1] {
		next, _ := m[p].(map[string]any)
		if next == nil {
			next = map[string]any{}
			m[p] = next
		}
		m = next
	}
	m[parts[len(parts)-1]] = v
}

// dropPath takes out the setting at a dotted path, and an object left empty by
// it, and reports whether there was one.
func dropPath(m map[string]any, path string) bool {
	parts := strings.Split(path, ".")
	if len(parts) == 1 {
		_, ok := m[path]
		delete(m, path)
		return ok
	}
	next, _ := m[parts[0]].(map[string]any)
	if next == nil || !dropPath(next, strings.Join(parts[1:], ".")) {
		return false
	}
	if len(next) == 0 {
		delete(m, parts[0])
	}
	return true
}
