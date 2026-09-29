---
name: phaethon
description: Use when a task needs a computer of its own — browsing and using websites (logging in, filling forms, reading pages, downloading files), running or testing desktop or GUI apps, installing and trying software without touching the user's machine, taking screenshots or screen recordings as evidence, or running several such sessions in parallel. Drives desks (small Linux VMs on the user's hollow hosts, or the user's own desktop when they have shared it) through bangboo's MCP tools, or the bangboo CLI when MCP is not available.
---

# phaethon: using a computer through bangboo

You have access to **desks**: small Linux VMs with a real screen (1280x800
unless you ask for another size), Chromium, a terminal, python3 and git, running on machines the user owns
(hollow hosts). bangboo is the bridge. Its tools appear as MCP tools named
`desk_new`, `computer`, `browser_open` and so on (in Claude Code:
`mcp__bangboo__desk_new`). Nothing you do on a desk touches the user's own
machine, with one exception the user has to grant: see "The user's own
desktop".

If those tools are not in your tool list, use the same tools from a shell:
`bangboo call TOOL key=value ...` (see "Without MCP" below).

## The loop

1. **Start**: `desk_new` (about 20 s). Give it a `name` if the user may want
   to come back to it. A name that is already running reattaches instead of
   starting a second desk. The new desk becomes the *current desk*, which
   every other tool uses unless you pass `desk`.
2. **Work**, picking the cheapest tool that can see what you need:
   - A web page → `browser_open`, `browser_read`, `browser_click`, `browser_type`.
   - Anything visual, or an app that is not a web page → `computer`.
   - Anything a command line does → `shell`.
   - Which window is where, and pasting long text → `windows`, `clipboard`.
3. **Deliver**: bring results back with `file_download`, `record_stop`, or
   text in your answer.
4. **Finish**: `desk_close`, unless the user asked to keep it. A desk holds
   memory on someone's machine until it is closed.

## Web pages: read, don't squint

`browser_open url=...` returns the page as **text plus a numbered list of
its links, buttons and fields**:

```
[2] input "Search Wikipedia"
[3] button "Search"
[14] link "Log in"
```

- `browser_type index=2 text="Alan Turing" submit=true` clicks the field,
  replaces its contents, types, and presses Enter.
- `browser_click index=14` clicks it with a real mouse click.
- Each answer shows the page **afterwards, renumbered**. Always use numbers
  from the latest answer, never from an older one.
- A long page comes in windows. The answer says `offset=N` for the next one.
  `browser_read offset=N` fetches it. Read the part you need rather than
  scrolling screenshots.
- `browser_open new_tab=true` keeps the current page open in its own tab.
- `browser_eval js=...` pulls structured data out in one call:
  `[...document.querySelectorAll('table tr')].map(r => r.innerText)`.
- Pass `screenshot=true` when layout matters: a chart, an image, a captcha,
  or checking that something *looks* right.

## The screen: the computer tool

`computer` uses the familiar computer-use actions: `screenshot`,
`left_click`, `double_click`, `triple_click`, `right_click`, `middle_click`,
`mouse_move`, `left_click_drag`, `left_mouse_down` / `left_mouse_up` (for
drags that need to pause on the way), `scroll`, `type`, `key`, `hold_key`,
`wait`, `zoom`, `cursor_position`.

- **Every action answers with a screenshot** taken once the screen has
  settled. Don't follow an action with a separate `screenshot`, because you
  already have it.
- Coordinates are pixels in those screenshots. Click the **centre** of the
  target.
- Click a field before typing into it. `type` types where the focus is.
- Keys use xdotool names: `Return`, `Tab`, `Escape`, `BackSpace`, `Delete`,
  `Page_Down`, `ctrl+l`, `ctrl+a`, `ctrl+shift+t`, `alt+F4`. Several keys
  separated by spaces are pressed in turn.
- Text too small to read → `zoom region=[x1,y1,x2,y2]`. The zoomed picture's
  coordinates are **not** clickable. The answer gives the formula to map
  them back.
- A page still loading → `wait duration=2`, which returns a fresh screenshot.
- The browser and the screen are the same Chromium. Mix freely: open with
  `browser_open`, handle a date picker with `computer`, then `browser_read`
  again.

## The command line: shell

`shell command="..."` runs as user `bot` in `/home/bot` (`cwd=` for
another directory), with passwordless `sudo`. Alpine packages come from `sudo apk add ...`, and pip and git work.
It returns the exit status, stdout and stderr.

- It waits up to `timeout_s` (default 60, max 600). For servers or GUI
  programs pass `background=true`. The window appears on the screen.
- Big output: redirect to a file and `file_read` it in pages.
- Prefer `shell` for downloads (`curl -LO`), unpacking, file inspection and
  scripting. It is far cheaper than doing the same through the screen.

## Windows and the clipboard

- `windows` lists the desk's open windows: each one's id, title, position,
  and which has focus. `windows activate=ID` brings one to the front, and
  `windows close=ID` closes it as its close button would. Reach for it when
  a program opened behind the browser or a dialog took the focus.
- `clipboard` with no arguments reads the desk's clipboard, e.g. what an
  app's "Copy" button put there. `clipboard text="..."` sets it. Then click
  the field and press `ctrl+v` (`computer action=key text=ctrl+v`), which is
  far faster than `type` for long text. Secret placeholders are not filled
  in on the clipboard: type those.

## Files

- `file_read path=...` pages through text files. Images come back as
  pictures.
- `file_write` writes text. `file_upload` copies a file **from the machine
  you run on** onto the desk.
- `file_download path=...` brings a file **to the machine you run on** and
  returns the local path. Browser downloads land in `/home/bot/Downloads`.

## Showing your work

- `record_start`, then do the task, then `record_stop` saves an MP4 locally
  and returns its path. This is useful when the user wants to see what
  happened.
- A final `computer action=screenshot` is good evidence for "it works".

## Several desks at once

Desks are independent machines, with no shared focus, clipboard or files.
For parallel work, start several with different names and pass
`desk=host/id` (or the name) on each call, or switch with `desk_use`.
`desk_list` shows them all. `hosts` shows each host's free memory. Put
heavy work (`mem_mb=2048`, `cpus=4`) on a host with room: `desk_new
host=NAME` picks it. `width` and `height` give a desk another screen size,
e.g. a phone-shaped 390x844.

## When something goes wrong

| Symptom | What to do |
|---|---|
| "no desk selected" | `desk_new`, or `desk_list` then `desk_use`. |
| "no host can take a linux desk right now" | Read the reasons it lists. An unbuilt image needs the user to run `phaethon sync`. |
| "host unreachable" | The host or its network (makima or Tailscale) is down. `hosts` shows which. Tell the user. |
| "the browser did not answer … out of memory" | Close tabs (`ctrl+w`), or start a desk with more `mem_mb`. |
| An element number fails | The page changed. `browser_read` again and use the new numbers. |
| Clicked but nothing happened | Look at the screenshot. The target may have moved, or it needed a double click or a wait. Try `zoom`. |
| A dialog or popup blocks the page | Deal with it through `computer`: `Escape`, or click its button. |
| "a person has taken this desk over" | The user is using it through the live view. Wait, look with `screenshot` or `browser_read`, and go on once `desk_list` no longer says they have it. |
| "refused: {{name}} is only for …" | The page is not the site the login belongs to. Check the address: it may be a lookalike. Don't try the secret anywhere else. |

## Logging in: the user's stored secrets

Never ask the user to paste a password into the chat. Call `secrets`
first: it lists the logins they stored, with the account, the fields and
the sites each one is for. It never shows the values. Then type
**placeholders**, and the host fills them in on the way to the desk:

- `browser_type index=1 text={{github.username}}`
- `browser_type index=2 text={{github}} submit=true` (the password)
- `browser_type index=5 text={{github.totp}}` (the current 2FA code, when a seed is stored)
- `{{name.FIELD}}` for any other field, e.g. an API token in `shell`:
  `curl -H 'Authorization: Bearer {{openai.token}}' ...`

The `computer` tool's `type` fills them in too. Click into the field first.
A login bound to a site is refused anywhere else: on another site, in a
terminal, in the address bar, or in `shell`. Anything the desk sends back
shows the placeholder, never the value. When there is no stored login, ask
the user to add one (`bangboo secret set NAME --username U --site HOST`),
or to log in themselves through the live view.

## When the user needs to step in

Some steps need a person: a CAPTCHA, a code sent to their phone, a payment
to confirm, a login with no stored secret. `desk_view` returns a link. Give
it to them and say what they need to do (`desk_view all=true` links to
every desk on the host at once). Once they click into the screen they have
the desk. Your actions on it are refused until they press Hand
back, but `screenshot`, `browser_read` and `desk_list` still work, so check
back with those. They can also just watch you work through the same link.

## The user's own desktop

Sometimes the task is on the user's real machine: their open apps, their
logged-in browser, a file on their screen. They can hand it to you for a
while with `bangboo local share` (they run it, not you; `--shell` adds
commands, `--files` adds files, `--for 2h` sets how long). While it is
shared it shows up as `local/desktop` in `desk_list` and `hosts`, and
`desk_new host=local` (or `desk_use desk=local`) makes it the current desk.
Nothing can be attached before that. If you are refused, tell the user what
to run; do not look for a way round it.

It is a real machine, so it is not the sandbox a VM desk is:

- What you close, type, click or delete is theirs. Work in what they asked
  about and leave every other window, tab and file alone. If you see
  something private on the way, do not read it out or act on it.
- Look before you act: `computer action=screenshot`, then small steps.
  Coordinates are in the screenshot, as on any desk.
- If they take it over (`desk_list` says "the user has it"), your actions are
  refused until they stop for a minute. Wait; screenshots still work.
- `browser_*` tools, `record_*` and secret placeholders (`{{name}}`) do not
  exist there: use `computer` on whatever browser is open, and ask the user
  to type passwords themselves.
- Keys are the same names as everywhere; `super` is Command on a Mac.
- `desk_close` ends the share. Do it as soon as you have finished.

## Without MCP: the bangboo CLI

The same tools work from any shell. The current desk is remembered between
calls. `--desk=NAME` (with the `=`) aims one call at another desk, and
`BANGBOO_DESK=NAME` does it for every call from that shell.

```sh
bangboo tools                                   # every tool and its arguments
bangboo call desk_new name=research
bangboo call browser_open url=example.com
bangboo call browser_type index=2 text="query" submit=true
bangboo call computer action=left_click coordinate=640,400
bangboo call shell command='ls -la ~/Downloads'
bangboo call desk_close
```

Screenshots are saved to files and the path is printed as
`[screenshot: /path.png]`. Read that file to see it. Write coordinates as
`640,400`, not `[640,400]`, which zsh mistakes for a glob. `--json` prints
the raw result.

## Setup, when there are no hosts

These commands are for the user, or for you with their go-ahead. They run
on the user's machine, not on a desk.

```sh
phaethon doctor              # what is installed, registered and reachable
phaethon scan                # hollows on the user's makima / Tailscale networks
phaethon host add MACHINE    # make any Linux box with KVM a host, over ssh
phaethon sync                # bring hosts, images and harnesses up to date
phaethon view                # watch every desk live, and take one over
phaethon local share         # give agents this desktop for a while (--shell, --files, --for)
phaethon secret set NAME --username U --site HOST   # a login agents type as {{NAME}}
```
