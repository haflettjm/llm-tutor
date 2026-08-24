-- llm-tutor Neovim plugin
-- Sends the current diff + a question to the knumble-tutor daemon and
-- displays the Socratic response in a floating window.
--
-- Setup (lazy.nvim):
--   { dir = "~/git/personal/llm-tutor/plugin/nvim", name = "llm-tutor" }
--
-- Commands:  :LlmAsk       ask a free-form question (no diff)
--            :LlmAskDiff   ask with the current git diff attached
--            :LlmStart     begin a session (tutor orients itself)
--            :LlmProgress  where you are in the current lesson plan
--            :LlmPlans     list the available lesson plan tracks
--            :LlmTrack     switch to a different track
--            :LlmNext      move on to the next concept
--            :LlmEnd       end the session and save what was learned
--            :LlmHealth    check the daemon is up and configured

local M = {}

-- Default config — override via M.setup({ socket = "..." })
local cfg = {
  socket = "/tmp/llm-tutor.sock",
  session_id = tostring(os.time()),
}

-- ── Helpers ───────────────────────────────────────────────────────────────────

local function get_diff()
  local out = vim.fn.system("git diff HEAD 2>/dev/null")
  return vim.v.shell_error == 0 and out or ""
end

local function detect_language()
  local ft = vim.bo.filetype
  return (ft ~= nil and ft ~= "") and ft or "unknown"
end

-- ── Response window ───────────────────────────────────────────────────────────

local function wrap_text(text, width)
  local lines = {}
  for _, paragraph in ipairs(vim.split(text, "\n")) do
    if #paragraph <= width then
      table.insert(lines, paragraph)
    else
      local pos = 1
      while pos <= #paragraph do
        local seg = paragraph:sub(pos, pos + width - 1)
        -- Back up to last space if mid-word
        if pos + width - 1 <= #paragraph then
          local last_space = seg:match(".*()%s")
          if last_space then
            seg = paragraph:sub(pos, pos + last_space - 2)
            pos = pos + last_space - 1
          else
            pos = pos + width
          end
        else
          pos = #paragraph + 1
        end
        table.insert(lines, seg)
      end
    end
  end
  return lines
end

local function type_label(response_type)
  local labels = {
    question    = "? Question",
    hint        = "~ Hint",
    explanation = "! Explanation",
    observation = "> Observation",
  }
  return labels[response_type] or response_type
end

local function show_response(resp)
  local max_w = math.min(80, vim.o.columns - 4)
  local lines = {}

  -- Header: response type + optional hint level
  local header = type_label(resp.response_type)
  if resp.hint_level and resp.hint_level > 0 then
    header = header .. string.format("  [level %d/3]", resp.hint_level)
  end
  if resp.concept_id and resp.concept_id ~= "" then
    header = header .. "  " .. resp.concept_id
  end
  table.insert(lines, header)
  table.insert(lines, string.rep("─", max_w - 2))

  -- Wrapped message body
  for _, l in ipairs(wrap_text(resp.message, max_w - 2)) do
    table.insert(lines, l)
  end

  -- Size the window
  local height = math.min(#lines, math.floor(vim.o.lines * 0.6))
  local width = 0
  for _, l in ipairs(lines) do
    width = math.max(width, #l)
  end
  width = math.max(width + 2, 40)

  local buf = vim.api.nvim_create_buf(false, true)
  vim.api.nvim_buf_set_lines(buf, 0, -1, false, lines)
  vim.api.nvim_set_option_value("modifiable", false, { buf = buf })
  vim.api.nvim_set_option_value("filetype", "markdown", { buf = buf })

  local win = vim.api.nvim_open_win(buf, true, {
    relative  = "editor",
    width     = width,
    height    = height,
    row       = math.floor((vim.o.lines - height) / 2),
    col       = math.floor((vim.o.columns - width) / 2),
    style     = "minimal",
    border    = "rounded",
    title     = " knumble-tutor ",
    title_pos = "center",
  })

  -- q / Escape closes; scroll with j/k
  local close = function() vim.api.nvim_win_close(win, true) end
  vim.keymap.set("n", "q",     close, { buffer = buf, nowait = true })
  vim.keymap.set("n", "<Esc>", close, { buffer = buf, nowait = true })
end

-- ── HTTP request over Unix socket ─────────────────────────────────────────────

-- request(method, path, body, on_ok) calls the daemon asynchronously.
-- on_ok receives the decoded response table.
local function request(method, path, body, on_ok)
  -- The URL goes on last, so optional flags can simply be appended.
  local cmd = { "curl", "-sf", "--unix-socket", cfg.socket, "-X", method }
  if body ~= nil then
    vim.list_extend(cmd, { "-H", "Content-Type: application/json", "-d", vim.fn.json_encode(body) })
  end
  table.insert(cmd, "http://localhost" .. path)

  local chunks = {}
  vim.fn.jobstart(cmd, {
    stdout_buffered = true,
    on_stdout = function(_, data)
      for _, chunk in ipairs(data) do
        if chunk ~= "" then
          table.insert(chunks, chunk)
        end
      end
    end,
    on_stderr = function(_, data)
      for _, line in ipairs(data) do
        if line ~= "" then
          vim.schedule(function()
            vim.notify("llm-tutor: " .. line, vim.log.levels.WARN)
          end)
        end
      end
    end,
    on_exit = function(_, code)
      vim.schedule(function()
        if code ~= 0 then
          vim.notify(
            string.format("llm-tutor: curl exited %d — is knumble-tutor running?", code),
            vim.log.levels.ERROR
          )
          return
        end

        local raw = table.concat(chunks, "")
        if raw == "" then
          vim.notify("llm-tutor: empty response from daemon", vim.log.levels.ERROR)
          return
        end

        local ok, resp = pcall(vim.fn.json_decode, raw)
        if not ok then
          vim.notify("llm-tutor: bad JSON: " .. raw, vim.log.levels.ERROR)
          return
        end

        if resp.error then
          vim.notify("llm-tutor: " .. resp.error, vim.log.levels.ERROR)
          return
        end

        on_ok(resp)
      end)
    end,
  })
end

local function do_query(message, diff)
  request("POST", "/tutor", {
    message    = message,
    diff       = diff or "",
    language   = detect_language(),
    concept_id = "",
    session_id = cfg.session_id,
  }, show_response)

  vim.notify("Asking tutor…", vim.log.levels.INFO)
end

-- ── Status rendering ──────────────────────────────────────────────────────────

local function bar(done, total)
  if not total or total == 0 then return "" end
  local width  = 20
  local filled = math.min(width, math.floor(done * width / total))
  return "  [" .. string.rep("#", filled) .. string.rep(".", width - filled) .. "]"
end

local function show_progress(p)
  local lines = {}
  if not p.track or p.track == "" then
    table.insert(lines, p.note or "No lesson plan selected yet.")
  else
    table.insert(lines, p.track_title or p.track)
    table.insert(lines, string.format("  %d/%d concepts demonstrated%s",
      p.demonstrated or 0, p.total or 0, bar(p.demonstrated or 0, p.total or 0)))
    if (p.learning or 0) > 0 then
      table.insert(lines, string.format("  %d in progress", p.learning))
    end
    if (p.review or 0) > 0 then
      table.insert(lines, string.format("  %d due for review", p.review))
    end
    table.insert(lines, string.format("  %d sessions so far", p.sessions or 0))
    if p.active_soul and p.active_soul ~= "" then
      table.insert(lines, "  teaching as: " .. p.active_soul)
    end
    if p.next_concept then
      local c = p.next_concept
      table.insert(lines, "")
      table.insert(lines, string.format("Up next — %s: %s (%d of %d)",
        c.id, c.title, p.position or 0, p.total or 0))
      if c.objective then table.insert(lines, "  Goal: " .. c.objective) end
      if c.evidence  then table.insert(lines, "  Done when: " .. c.evidence) end
    end
    if p.note and p.note ~= "" then
      table.insert(lines, "")
      table.insert(lines, p.note)
    end
  end
  show_response({ message = table.concat(lines, "\n"), response_type = "observation" })
end

local function show_plans(p)
  local lines = { "Lesson plan tracks:", "" }
  for _, s in ipairs(p.plans or {}) do
    local marker = (s.id == p.active) and "* " or "  "
    table.insert(lines, marker .. s.id)
    local detail = "      " .. (s.title or "")
    if (s.concepts or 0) > 0 then
      detail = detail .. string.format(" — %d concepts", s.concepts)
    end
    table.insert(lines, detail)
  end
  if p.active and p.active ~= "" then
    table.insert(lines, "")
    table.insert(lines, "* = active track")
  end
  show_response({ message = table.concat(lines, "\n"), response_type = "observation" })
end

-- ── Public commands ───────────────────────────────────────────────────────────

function M.ask(message)
  if message and message ~= "" then
    do_query(message, nil)
  else
    vim.ui.input({ prompt = "Ask tutor: " }, function(input)
      if input and input ~= "" then
        do_query(input, nil)
      end
    end)
  end
end

function M.ask_with_diff(message)
  local diff = get_diff()
  if message and message ~= "" then
    do_query(message, diff)
  else
    vim.ui.input({ prompt = "Ask tutor (with diff): " }, function(input)
      if input and input ~= "" then
        do_query(input, diff)
      end
    end)
  end
end

function M.progress()
  request("GET", "/progress", nil, show_progress)
end

function M.plans()
  request("GET", "/plans", nil, show_plans)
end

function M.switch_track(track)
  if track and track ~= "" then
    request("POST", "/track", { track = track }, show_progress)
    return
  end
  -- No track given: offer the list to pick from.
  request("GET", "/plans", nil, function(p)
    local ids = {}
    for _, s in ipairs(p.plans or {}) do table.insert(ids, s.id) end
    if #ids == 0 then
      vim.notify("llm-tutor: no lesson plans found", vim.log.levels.WARN)
      return
    end
    vim.ui.select(ids, { prompt = "Switch to track:" }, function(choice)
      if choice then request("POST", "/track", { track = choice }, show_progress) end
    end)
  end)
end

-- next/end go through the model: moving on should open the concept Socratically,
-- and ending a session needs the tutor to write the session note itself.
function M.start_session(what)
  do_query("/start " .. (what or ""), nil)
end

function M.next_concept()
  do_query("/next", nil)
end

function M.end_session(note)
  do_query("/end " .. (note or ""), nil)
end

function M.health()
  request("GET", "/health", nil, function(h)
    vim.notify(string.format(
      "llm-tutor: %s · harness=%s · soul=%s · %d lesson plans",
      h.status, h.harness, h.active_soul or "none", h.lesson_plans or 0))
  end)
end

-- ── Setup ─────────────────────────────────────────────────────────────────────

function M.setup(user_cfg)
  if user_cfg then
    cfg = vim.tbl_deep_extend("force", cfg, user_cfg)
  end

  vim.api.nvim_create_user_command("LlmAsk", function(opts)
    M.ask(opts.args ~= "" and opts.args or nil)
  end, { nargs = "?", desc = "Ask the Socratic tutor a question" })

  vim.api.nvim_create_user_command("LlmAskDiff", function(opts)
    M.ask_with_diff(opts.args ~= "" and opts.args or nil)
  end, { nargs = "?", desc = "Ask the tutor and attach the current git diff" })

  vim.api.nvim_create_user_command("LlmStart", function(opts)
    M.start_session(opts.args)
  end, { nargs = "?", desc = "Tutor: begin a session" })
  vim.api.nvim_create_user_command("LlmProgress", M.progress,
    { desc = "Tutor: where you are in the current lesson plan" })
  vim.api.nvim_create_user_command("LlmPlans", M.plans,
    { desc = "Tutor: list the available lesson plan tracks" })
  vim.api.nvim_create_user_command("LlmTrack", function(opts)
    M.switch_track(opts.args)
  end, { nargs = "?", desc = "Tutor: switch lesson plan track" })
  vim.api.nvim_create_user_command("LlmNext", M.next_concept,
    { desc = "Tutor: move on to the next concept" })
  vim.api.nvim_create_user_command("LlmEnd", function(opts)
    M.end_session(opts.args)
  end, { nargs = "?", desc = "Tutor: end the session and save notes" })
  vim.api.nvim_create_user_command("LlmHealth", M.health,
    { desc = "Tutor: check the daemon is up" })

  vim.keymap.set("n", "<leader>ta", M.ask,           { desc = "Tutor: ask" })
  vim.keymap.set("n", "<leader>td", M.ask_with_diff, { desc = "Tutor: ask with diff" })
  vim.keymap.set("n", "<leader>ts", M.start_session, { desc = "Tutor: start session" })
  vim.keymap.set("n", "<leader>tp", M.progress,      { desc = "Tutor: progress" })
  vim.keymap.set("n", "<leader>tn", M.next_concept,  { desc = "Tutor: next concept" })
end

return M
