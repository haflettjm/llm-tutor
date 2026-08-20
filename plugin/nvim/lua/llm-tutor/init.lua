-- llm-tutor Neovim plugin
-- Sends the current diff + a question to the knumble-tutor daemon and
-- displays the Socratic response in a floating window.
--
-- Setup (lazy.nvim):
--   { dir = "~/git/personal/llm-tutor/plugin/nvim", name = "llm-tutor" }
--
-- Commands:  :LlmAsk    ask a free-form question (no diff)
--            :LlmAskDiff  ask with the current git diff attached

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

local function do_query(message, diff)
  local req = {
    message    = message,
    diff       = diff or "",
    language   = detect_language(),
    concept_id = "",
    session_id = cfg.session_id,
  }

  local body = vim.fn.json_encode(req)

  local cmd = {
    "curl", "-sf",
    "--unix-socket", cfg.socket,
    "-X", "POST",
    "-H", "Content-Type: application/json",
    "-d", body,
    "http://localhost/tutor",
  }

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

        show_response(resp)
      end)
    end,
  })

  vim.notify("Asking tutor…", vim.log.levels.INFO)
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

  vim.keymap.set("n", "<leader>ta", M.ask,           { desc = "Tutor: ask" })
  vim.keymap.set("n", "<leader>td", M.ask_with_diff, { desc = "Tutor: ask with diff" })
end

return M
