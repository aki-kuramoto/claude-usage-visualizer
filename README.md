# claude-usage-visualizer

> **Note:** The second half of this document is available in Japanese (日本語).

A small desktop app that shows how much of your Claude Code plan limits you have used (the five-hour limit and the weekly limits) as bars in a small window, and keeps a record of the values it reads.

- On macOS (13 or later) and Windows (10 version 1809 or later, and 11), it shows a window.
- Linux has no window yet. With `-headless`, the app only reads and records usage.

This is not an Anthropic product, and is not affiliated with or endorsed by Anthropic.

## Requirements

- Claude Code (the `claude` command), logged in with a Claude subscription. The app reads the usage through it. It looks in `PATH` and where the installers put it; on Windows, `%USERPROFILE%\.local\bin\claude.exe` (the native installer) and `%APPDATA%\npm\claude.cmd` (npm). Elsewhere, set its path in the settings. The copy of Claude Code inside Claude Desktop is not looked for: it changes place with every update, and it is not logged in on its own. If you only have Claude Desktop, install the command too, then start `claude` once and log in:

  ```powershell
  # Windows (PowerShell)
  irm https://claude.ai/install.ps1 | iex
  ```

  ```bash
  # macOS, Linux
  curl -fsSL https://claude.ai/install.sh | bash
  ```

  The app notices the new `claude` by itself within a minute; it need not be restarted.
- To build: Go 1.24 or later. On macOS, also the Xcode command line tools. On Windows, nothing else (no C compiler).

## Install (macOS)

```bash
go run ./tools/local-install
```

This runs `go vet` and the tests, then builds `~/Applications/ClaudeUsageVisualizer.app` and signs it ad hoc. A copy that is already running keeps running until you quit it. `-q` skips vet and the tests; `-dir` installs somewhere else.

## Install (Windows)

```bash
go run ./tools/local-install
```

This runs `go vet` and the tests, then builds `claude-usage-visualizer.exe` into `%USERPROFILE%\programs\claude-usage-visualizer\`. Start it from there (or make a shortcut to it yourself; the installer makes none). A copy that is already running keeps running until you quit it; the old executable is moved aside and removed at the next install. `-q` skips vet and the tests; `-dir` installs somewhere else, for example `-dir D:\tools\cuv`.

Elsewhere, or without a window:

```bash
go run ./cmd/claude-usage-visualizer -headless
```

## The window

- One bar for each limit, with the reset time and the time left until it.
- The vertical line across a bar shows how much of the period until the reset has passed. A bar that reaches past the line means you are using more than the time elapsed would suggest.
- The thin cuts in a bar are calendar boundaries: local midnight for the weekly limits, the hour (14:00, 15:00, ...) for the five-hour limit. The small dots just below a bar divide the period into equal parts from the reset time (seven for the weekly limits, five for the five-hour limit). The period starts at the reset time, so the two kinds of ticks do not line up.
- The five-hour limit starts with the first use, so while nothing has been used Claude Code gives no reset time for it. The bar is then drawn as if the period started now: the line at the left edge, and the ticks where they would fall.
- The small triangle above a weekly bar is the time elapsed counted only by the days you use Claude; the small triangle below it is the time elapsed judged by the days still ahead. Both come from a file you can edit (see "Days" below).
- When a reading fails, the bars turn gray, and the window says what went wrong and what to do.
- The title bar is drawn by the app itself, in the style of the classic Mac OS. The box with an X on the left closes the window; the box on the right switches to the **compact view**. Drag the title bar to move the window.
- **Compact view**: like the window shade of the classic Mac OS, the window keeps its width and shrinks to its title bar, and the title shows the five-hour limit and the weekly limit for all models, such as "5h 21%  ·  week 42%". When nothing can be read, the title says only that. Switch back with the same box, a double-click on the title bar, or View > Compact View (Cmd+Shift+C on macOS, Ctrl+Shift+C on Windows).
- The menu bar inside the window (Usage, View, Settings) has Read Now, Compact View and Settings. On macOS, the same items are also in the menu bar at the top of the screen.
- Two looks: Platinum (in the style of Mac OS 8, the default) and System 7. Choose in the settings.
- On Windows, the app also has an icon in the notification area (the system tray). Its tooltip shows the same line as the compact view. Clicking it shows the window; its menu has Read Now, Settings and Quit. Closing the window leaves the app running there (see `close_action` below), and Quit in that menu ends it. Ctrl+R reads now, Ctrl+, opens the settings and Ctrl+W closes the window. The text is in Yu Gothic UI in Japanese and Segoe UI in English, drawn at each monitor's scale.

## How the usage is read

The app keeps one `claude` process running (`claude -p --input-format stream-json --output-format stream-json ...`), sends it `/usage` at each reading, and reads the percentages from the answer.

- `/usage` is a local command that does not call a model, so reading does not use up your plan.
- Each reading still takes about three seconds of CPU in `claude`, because `/usage` also analyses your local session history. A shorter interval uses more CPU.
- Your MCP servers are not started (`--strict-mcp-config`). Your SessionStart hooks run whenever `claude` starts.
- The answer of `/usage` is not a published interface. A Claude Code update may change it, and the app may then fail to read it until it is updated.

## Files

Under your home directory (`$HOME` on macOS and Linux, `%USERPROFILE%` on Windows), in `.app-data/claude-usage-visualizer/`:

| File | Content |
|---|---|
| `settings.setay` | The settings. Also editable from the settings window. |
| `days.setay` | The days (see "Days" below). |
| `logs/usage-YYYY-MM-DD.jsonl` | The usage record, one line per reading. |
| `logs/ops-YYYY-MM-DD.jsonl` | The operation log: what the app did and what happened. |
| `work/` | The working directory where the app runs `claude`. It stays empty. |
| `window.json` | Windows only: where the window was, to open it there again. |

The log files are split by UTC date. The app never deletes them.

## Settings

Change them in the settings window (Cmd+, on macOS, Ctrl+, on Windows) or in `settings.setay`, which is in the [setay](https://github.com/aki-kuramoto/setay-go) format. You may edit the file by hand; saving from the settings window rewrites only the values that changed and keeps your comments.

| Key | Default | Meaning |
|---|---|---|
| `interval_minutes` | `5` | How often usage is read, in minutes (1 to 60). |
| `claude_path` | `''` | Where the `claude` command is. Empty means find it. |
| `always_on_top` | `false` | Keep the window above the windows of other applications. |
| `language` | `'system'` | The language of the window: `'system'` (as the OS is set), `'ja'` or `'en'`. |
| `close_action` | `'ask'` | What closing the window does on macOS and Windows: `'ask'` (ask whether to quit), `'close'` (close it and keep recording; on Windows the app stays in the notification area) or `'quit'`. |
| `login_expiry_check` | `true` | Check when the login expires (see below). |
| `start_compact` | `false` | Start in the compact view. Switching while running is not written back here. |
| `theme` | `'platinum'` | The look of the window: `'platinum'` or `'system7'`. |

## Days

`days.setay` says how much you use Claude on each day of the week, and lists exceptions such as holidays. The triangles at the weekly bars are worked out from it. When there is none, the app writes one with weekdays weighing 9 and weekends 1. See [docs/days-setay.md](docs/days-setay.md) for how to write it; handing that document to an agent is enough to have it set up for you.

## When claude is not logged in

Without a login, `/usage` shows no limits. The app recognizes this (with `claude auth status`), says "Claude Code isn't logged in", puts a "!" badge on its Dock icon and shows a notification (macOS asks for permission the first time). On Windows, the badge is on the icon in the notification area, and the notification comes from that icon.

The "Log In…" button opens a terminal (on Windows, a console window) that runs `claude auth login`. The login itself happens in your browser; the app never logs in for you. Once you are logged in again, the app notices within a minute and resumes reading.

Logging in may take two attempts: in one case, the first authentication in the browser seemed to do nothing, and the second one worked.

## Login expiry check

Claude Code's login expires. When it is close to expiring, Claude Code says so ("Your login expires in 3 days · run /login to renew") right after the interactive screen starts, but only there: neither `claude -p` nor `claude auth status` reports it.

So about once a day (and shortly after starting, and right after you log in again), the app starts the interactive `claude` on a pseudo terminal for a few seconds and reads whether that warning is on the screen. If it is, the window says how many days are left and offers the "Log In…" button.

- Nothing is typed, so no conversation starts and nothing is recorded.
- Each check takes a few seconds of CPU and runs your SessionStart hooks. MCP servers are not started.
- It reads text on a screen, so a change in Claude Code's screen may break it.
- It works on macOS, Linux and Windows (on Windows through the pseudo console, ConPTY).

To turn it off, clear "Check when the login expires" in the settings, or write `login_expiry_check = false;` in `settings.setay`.

### Trusting the working directory once

When the interactive `claude` starts in a directory you have not trusted yet, it first asks whether you trust it. The app's working directory (`~/.app-data/claude-usage-visualizer/work/`) starts out untrusted, so the question comes before the warning the app is looking for.

That question is Claude Code's safety check, and your answer is written to Claude Code's settings, so the app never answers it for you. Instead, the window says the directory needs to be trusted once, with a "Trust the Folder…" button. The button opens a terminal (on Windows, a console window) that starts `claude` in the working directory: answer "Yes, I trust this folder" (the question starts on "No, exit", so press the down arrow before Enter), and leave with `/exit` once `claude` has started. The app looks again a few times over the next minutes (on Windows also as soon as the console closes), and the button goes away once the folder is trusted. The directory belongs to this app and stays empty. If you would rather not trust it, turn the check off instead.

## The usage record

`logs/usage-YYYY-MM-DD.jsonl` has one JSON object per line, one line per reading:

```json
{"v":1,"id":"019a1b2c-...","m":"stream-json-resident","at":"2026-10-22T09:00:19+09:00","ok":true,
 "raw":{"source":"You are currently using your subscription to power your Claude Code usage",
        "limits":[{"label":"Current session","usage":"19% used","resets_at":"Oct 22 at 10:19am (Asia/Tokyo)"}]},
 "norm":{"source":"subscription",
         "limits":[{"label":"Current session","used_pct":19,"resets_at":"2026-10-22T01:19:00Z"}]},
 "error":null}
```

(Shown on several lines here; in the file, each record is one line.)

- `v`: the version of this format. `id`: a UUID v7. `m`: how the reading was taken. `at`: when, with the local offset.
- `raw`: the text of `/usage` as printed. `norm`: the same values in fixed forms (percentages as numbers, reset times in UTC). Keeping both lets a mistake in the conversion be corrected later from the raw text.
- `ok` is `true` when `norm` could be made. Otherwise `error` has `stage`, `reason` and `detail`.
- The limit labels are kept as Claude Code prints them (`Current session`, `Current week (all models)`, ...).
- To find the latest values, use the last line whose `ok` is `true`.

## Development

```bash
go test ./...
```

- `-base <dir>` keeps the settings and records in another directory, apart from the ones you use.
- With the environment variable `CUV_SNAPSHOT_DIR=<dir>`, the app writes PNGs of its windows as it draws them, in both the front and the background look. With `CUV_HIDDEN=1` as well, it shows no window, never comes to the front and has no Dock icon (on Windows, no icon in the notification area), so the drawing can be checked without disturbing your screen. On Windows, the drop-down lists of the settings window come out empty in these PNGs; on the screen they are filled.
- `CUV_WINDOW_AT=x,y` puts the window's top-left corner there when it is first shown (on macOS in the coordinates of System Events: from the top left of the main screen, y growing downwards; on Windows in pixels of the virtual screen).
- `go run ./tools/genicon` draws the icon again. It is a usage gauge with a troubled face `>_<` made of terminal prompt characters; it does not use Anthropic's logo or characters. It also writes `cmd/claude-usage-visualizer/rsrc_windows_*.syso`, the Windows icon and manifest that `go build` links in. `AppIcon.icns` needs `iconutil`, which only macOS has; on other systems it is left as it is.
- On Windows, `go run ./cmd/claude-usage-visualizer` also opens a console window next to the app; the installer builds with `-ldflags=-H=windowsgui`, which does not. Only one copy runs for each data directory (`-base`); starting another shows the window of the one running.

## License

[MIT](LICENSE.md)

---

# claude-usage-visualizer (日本語)

Claude Code のプランの使用状況 (5 時間枠と週の枠の使用率) を小さなウィンドウにバーで表示し、読み取った値を記録し続けるアプリです。

- macOS (13 以降) と Windows (10 のバージョン 1809 以降と 11) では、ウィンドウで表示します。
- Linux では、まだウィンドウがありません。`-headless` を付けると、取得と記録だけを行います。

Anthropic の製品ではなく、Anthropic とは関係がありません。

## 必要なもの

- Claude Code (`claude` コマンド) に、Claude のサブスクリプションでログインしていること。アプリはこれを通して使用量を読みます。`PATH` と、インストーラーが置く場所から探します。Windows では `%USERPROFILE%\.local\bin\claude.exe` (公式のインストーラー) と `%APPDATA%\npm\claude.cmd` (npm) です。ほかの場所にあるときは、設定で場所を指定してください。Claude Desktop の中にある Claude Code は探しません。更新のたびに場所が変わるうえ、それ単体ではログインしていないためです。Claude Desktop しか入れていないときは、コマンドも入れて、一度 `claude` を起動してログインしてください。

  ```powershell
  # Windows (PowerShell)
  irm https://claude.ai/install.ps1 | iex
  ```

  ```bash
  # macOS、Linux
  curl -fsSL https://claude.ai/install.sh | bash
  ```

  入れた `claude` は、アプリが 1 分以内に自分で見つけます。アプリを起動し直す必要はありません。
- ビルドには Go 1.24 以降が要ります。macOS では Xcode のコマンドラインツールも要ります。Windows ではほかに何も要りません (C コンパイラーも不要です)。

## 入れ方 (macOS)

```bash
go run ./tools/local-install
```

`go vet` とテストを行ったあと、`~/Applications/ClaudeUsageVisualizer.app` を作り、アドホック署名をします。動いている古い版は、終了するまでそのまま動きます。`-q` で vet とテストを省き、`-dir` でほかの場所に入れます。

## 入れ方 (Windows)

```bash
go run ./tools/local-install
```

`go vet` とテストを行ったあと、`%USERPROFILE%\programs\claude-usage-visualizer\` に `claude-usage-visualizer.exe` を作ります。そこから起動してください (ショートカットは作らないので、必要なら自分で作ってください)。動いている古い版は、終了するまでそのまま動きます。古い exe は脇に移され、次に入れるときに消えます。`-q` で vet とテストを省き、`-dir` でほかの場所に入れます (例: `-dir D:\tools\cuv`)。

ほかの OS や、ウィンドウなしで動かすときは次のとおりです。

```bash
go run ./cmd/claude-usage-visualizer -headless
```

## 表示

- 枠ごとに、使用率のバー、リセット時刻、リセットまでの残り時間を出します。
- バーを縦に貫く線は、リセットまでの期間のうち、どれだけ時間が過ぎたかを示します。バーがこの線より右まで伸びていれば、時間の割に使いすぎです。
- バーの細い切れ目は、カレンダーの区切りです。週の枠ではローカルの日付の変わり目 (0 時)、5 時間枠では時計の正時 (14:00、15:00 など) です。バーのすぐ下の小さな点は、期間をリセットの時刻から等分した区切りです (週の枠は 7 等分、5 時間枠は 5 等分)。期間はリセットの時刻から始まるので、2 種類の区切りはずれます。
- 5 時間枠は最初に使ったときに始まるので、まだ使っていない間は Claude Code がリセット時刻を出しません。そのときは、いま始まったものとしてバーを描きます (線は左端、区切りはいま始まった場合の位置)。
- 週の枠のバーの上の小さな三角は、Claude を使う日だけで数えたときの経過です。バーの下の小さな三角は、これから先の日の重みから見た経過です。どちらも、自分で書き換えられるファイルから計算します (下の「日の設定」)。
- 取得に失敗したときは、バーを灰色にして、何が起きていて何をすればよいかを出します。
- タイトルバーは、昔の Mac OS の雰囲気でアプリ自身が描いています。左の X の箱で閉じ、右の箱で**小さく表示**に切り替えます。タイトルバーをドラッグすると移動できます。
- **小さく表示**: 昔の Mac OS のウインドウシェードのように、ウィンドウが幅はそのままでタイトルバーだけになり、タイトルに 5 時間枠と全モデルの週の枠 (「5h 21%  ·  週 42%」など) が出ます。取得できていないときは、タイトルはそのことだけを出します。同じ箱、タイトルバーのダブルクリック、「表示」→「小さく表示」(macOS は Cmd+Shift+C、Windows は Ctrl+Shift+C) で戻せます。
- ウィンドウの中のメニュー (「使用量」「表示」「設定」) には、今すぐ取得、小さく表示、設定があります。macOS では、同じものが画面上部のメニューバーにもあります。
- 見た目は、プラチナ (Mac OS 8 風、既定) と System 7 風の 2 つから設定で選べます。
- Windows では、通知領域 (タスクトレイ) にもアイコンを出します。マウスを載せると、小さく表示と同じ 1 行が出ます。クリックするとウィンドウを出し、右クリックのメニューには今すぐ取得、設定、終了があります。ウィンドウを閉じてもアプリは通知領域で動き続け (下の `close_action`)、このメニューの終了で終わります。Ctrl+R で今すぐ取得、Ctrl+, で設定、Ctrl+W でウィンドウを閉じます。文字は日本語では Yu Gothic UI、英語では Segoe UI で、モニターごとの拡大率に合わせて描きます。

## 使用量の取り方

アプリは `claude` を 1 つ起こしっぱなしにし (`claude -p --input-format stream-json --output-format stream-json ...`)、取得のたびに `/usage` を送って、返ってきた答えから使用率を読みます。

- `/usage` はモデルを呼ばない手元のコマンドなので、取得でプランの使用量は減りません。
- ただし 1 回ごとに `claude` が CPU を約 3 秒使います。`/usage` が手元の会話の記録の分析も行うためです。取得間隔を短くすると、そのぶん CPU を使います。
- ユーザーの MCP サーバーは起動しません (`--strict-mcp-config`)。SessionStart のフックは `claude` を起動するたびに実行されます。
- `/usage` の答えは公開された形式ではありません。Claude Code の更新で変わると、アプリが対応するまで読めなくなることがあります。

## ファイルの置き場所

ホームディレクトリ (macOS と Linux は `$HOME`、Windows は `%USERPROFILE%`) の下の `.app-data/claude-usage-visualizer/` です。

| ファイル | 中身 |
|---|---|
| `settings.setay` | 設定。設定画面からも変えられます。 |
| `days.setay` | 日の設定 (下の「日の設定」)。 |
| `logs/usage-YYYY-MM-DD.jsonl` | 使用量の記録。1 回の取得が 1 行です。 |
| `logs/ops-YYYY-MM-DD.jsonl` | 動作の記録。アプリが何をし、何が起きたか。 |
| `work/` | アプリが `claude` を動かす作業フォルダー。中身は空のままです。 |
| `window.json` | Windows のみ。ウィンドウの位置。次に開くときにその位置に出します。 |

記録のファイルは UTC の日付で分かれます。アプリが消すことはありません。

## 設定

設定画面 (macOS は Cmd+,、Windows は Ctrl+,) か `settings.setay` で変えられます。`settings.setay` は [setay](https://github.com/aki-kuramoto/setay-go) 形式です。手で書き換えてもよく、設定画面から保存したときは、値の変わった項目だけが書き換わります (コメントは残ります)。

| 項目 | 既定 | 意味 |
|---|---|---|
| `interval_minutes` | `5` | 使用量を取得する間隔 (分、1〜60)。 |
| `claude_path` | `''` | `claude` コマンドの場所。空なら自動で探します。 |
| `always_on_top` | `false` | ウィンドウをほかのアプリのウィンドウより常に手前に出します。 |
| `language` | `'system'` | 画面の言語。`'system'` (OS の設定に従う)、`'ja'`、`'en'`。 |
| `close_action` | `'ask'` | macOS と Windows でウィンドウを閉じたとき。`'ask'` (終了するか訊く)、`'close'` (閉じるだけで記録は続ける。Windows では通知領域に残る)、`'quit'` (終了する)。 |
| `login_expiry_check` | `true` | ログインの期限を確かめます (下の「ログインの期限の確認」)。 |
| `start_compact` | `false` | 起動した直後に小さく表示します。動いている間の切り替えは、ここには書き戻しません。 |
| `theme` | `'platinum'` | ウィンドウの見た目。`'platinum'` か `'system7'`。 |

## 日の設定

`days.setay` には、曜日ごとにどれくらい Claude を使うかと、祝日などの例外の日を書きます。週の枠のバーの三角は、この設定から計算します。ファイルがなければ、アプリが平日の重み 9、土日の重み 1 の内容で作ります。書き方は [docs/days-setay.md](docs/days-setay.md) を見てください。エージェントに設定を頼むときも、この文書を渡せば足ります。

## ログインしていないとき

ログインしていないと、`/usage` は枠を返しません。アプリはそれを (`claude auth status` で) 見分けて「Claude Code にログインしていません」と出し、Dock のアイコンに「!」を付け、通知で知らせます (通知は macOS が初回に許可を求めます)。Windows では、通知領域のアイコンに「!」を付け、そのアイコンから通知を出します。

「ログインする…」ボタンを押すと、ターミナル (Windows ではコンソールのウィンドウ) が開いて `claude auth login` が動きます。ログインそのものはブラウザで行うもので、アプリが代わりに行うことはありません。ログインし直すと、アプリは 1 分以内に気づいて取得を再開します。

ログインは 2 回ほどやり直す必要があるかもしれません。1 回目はブラウザで認証を済ませても何も起こらず、2 回目でログインできたことがありました。

## ログインの期限の確認

Claude Code のログインには期限があります。期限が近づくと、Claude Code は対話画面の起動直後に「Your login expires in 3 days · run /login to renew」と知らせますが、この知らせは対話画面にしか出ません。`claude -p` にも `claude auth status` にも出てきません。

そこでアプリは、1 日 1 回ほど (起動の少しあとと、ログインし直した直後にも)、対話モードの `claude` を疑似端末で数秒だけ起動し、画面にこの知らせが出ていないかを読みます。出ていれば、期限まであと何日かと「ログインする…」ボタンを出します。

- 何も入力しないので、会話は始まらず、記録も残りません。
- 1 回ごとに数秒の CPU を使い、ユーザーの SessionStart のフックが実行されます。MCP サーバーは起動しません。
- 画面の文字を読む仕組みなので、Claude Code の画面が変わると読めなくなることがあります。
- macOS、Linux、Windows で動きます (Windows では疑似コンソール ConPTY を使います)。

この確認が要らなければ、設定画面の「ログインの期限を確かめる」を外すか、`settings.setay` で `login_expiry_check = false;` にしてください。

### 作業フォルダーを一度だけ信頼してもらう理由

まだ信頼していないフォルダーで対話モードの `claude` を起動すると、Claude Code はまずそのフォルダーを信頼するかを確認します。アプリの作業フォルダー (`~/.app-data/claude-usage-visualizer/work/`) も最初は信頼されていないので、この確認が先に出て、アプリが探している知らせにたどり着けません。

この確認は Claude Code の安全の仕組みで、答えは Claude Code の設定に書き込まれるので、アプリが代わりに答えることはしません。代わりにウィンドウに、作業フォルダーを一度だけ信頼する必要があることと「フォルダーを信頼する…」ボタンを出します。ボタンを押すとターミナル (Windows ではコンソールのウィンドウ) が開き、作業フォルダーで `claude` が起動します。「Yes, I trust this folder」と答え (最初は「No, exit」が選ばれているので、↓ キーで移ってから Enter を押します)、`claude` が起動したら `/exit` で終了してください。アプリはそのあと数分の間に何度か確かめ直し (Windows ではコンソールを閉じた時点でも確かめます)、信頼されていればボタンは消えます。このフォルダーはこのアプリ専用で、中身は空のままです。信頼したくない場合は、代わりに期限の確認を外してください。

## 使用量の記録

`logs/usage-YYYY-MM-DD.jsonl` は、1 回の取得が 1 行の JSON です。

```json
{"v":1,"id":"019a1b2c-...","m":"stream-json-resident","at":"2026-10-22T09:00:19+09:00","ok":true,
 "raw":{"source":"You are currently using your subscription to power your Claude Code usage",
        "limits":[{"label":"Current session","usage":"19% used","resets_at":"Oct 22 at 10:19am (Asia/Tokyo)"}]},
 "norm":{"source":"subscription",
         "limits":[{"label":"Current session","used_pct":19,"resets_at":"2026-10-22T01:19:00Z"}]},
 "error":null}
```

(ここでは複数行に分けていますが、ファイルでは 1 件が 1 行です。)

- `v` はこの形式の版、`id` は UUID v7、`m` は取得の方法、`at` は取得した時刻 (ローカルのオフセット付き) です。
- `raw` は `/usage` の文字列そのまま、`norm` は同じ値を決まった形にしたもの (使用率は数、リセット時刻は UTC) です。両方を残すので、変換に間違いがあっても、あとで元の文字列から直せます。
- `norm` を作れたときは `ok` が `true` です。そうでないときは、`error` に `stage`、`reason`、`detail` が入ります。
- 枠の名前は、Claude Code が出したとおり (`Current session`、`Current week (all models)` など) で持ちます。
- 最新の値は、`ok` が `true` の最後の行から取ってください。

## 開発

```bash
go test ./...
```

- `-base <ディレクトリ>` を付けると、設定と記録を普段のものとは別の場所に置きます。
- 環境変数 `CUV_SNAPSHOT_DIR=<ディレクトリ>` を付けると、アプリは描くたびにウィンドウを PNG で書き出します (前面と後ろの両方の見た目)。`CUV_HIDDEN=1` も付けると、ウィンドウを画面に出さず、前面にも出ず、Dock にもアイコンを出さない (Windows では通知領域にも出さない) ので、画面を乱さずに描画を確かめられます。Windows では、設定画面のドロップダウンが PNG では空になりますが、画面では入っています。
- `CUV_WINDOW_AT=x,y` で、最初にウィンドウを出す位置 (左上の角) を指定できます。macOS では座標は System Events と同じで、メインの画面の左上が原点、下向きが正です。Windows では仮想画面のピクセルです。
- `go run ./tools/genicon` でアイコンを描き直せます。使用量のゲージと、ターミナルのプロンプトの文字で描いた困った顔 `>_<` で、Anthropic のロゴやキャラクターは使っていません。Windows の exe に入れるアイコンと manifest (`cmd/claude-usage-visualizer/rsrc_windows_*.syso`、`go build` が自動で取り込みます) もここで書き出します。`AppIcon.icns` には macOS にしかない `iconutil` が要るので、ほかの OS ではそのままにします。
- Windows で `go run ./cmd/claude-usage-visualizer` とすると、アプリの横にコンソールのウィンドウも開きます。インストーラーは `-ldflags=-H=windowsgui` でビルドするので開きません。データのディレクトリ (`-base`) ごとに 1 つしか動かず、もう 1 つ起動すると、動いている方のウィンドウを出します。

## ライセンス

[MIT](LICENSE.md)
