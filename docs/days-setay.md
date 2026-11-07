# Days (days.setay)

> **Note:** The second half of this document is available in Japanese (日本語).

`days.setay` tells claude-usage-visualizer how much you use Claude on each day, so that it can show two small triangles at each weekly bar:

- The triangle **above** the bar, pointing down: how much of the period until the reset has passed, counting only the days you use Claude.
- The triangle **below** the bar, pointing up: how far along you can consider yourself, judged by the days still ahead.

You write how much you use Claude on each day of the week, and exceptions such as holidays and days off. You can write it yourself, or hand this document to an agent and ask it to.

## Where it is

`.app-data/claude-usage-visualizer/days.setay` under your home directory (`~/.app-data/claude-usage-visualizer/days.setay` on macOS, `%USERPROFILE%\.app-data\claude-usage-visualizer\days.setay` on Windows).

When the app starts and there is no such file, it writes one: weekdays (`weekday`) weigh 9, weekends (`weekend`) weigh 1, and there are no exceptions (with commented-out examples). An existing file is never overwritten. If you delete it, it is written again the next time the app starts.

The app picks up changes while it runs, so there is no need to restart it (the window shows them within 30 seconds). If the file is deleted while the app runs, the triangles just disappear.

## Example

```
{
	# The kinds of days, each with a weight. Name them as you like.
	kinds = {
		work = 1;
		off = 0;
		light = 0.3;
	};

	# The kind of each day of the week. Write all seven.
	weekdays = {
		mon = 'work'; tue = 'work'; wed = 'work'; thu = 'work'; fri = 'work';
		sat = 'off'; sun = 'off';
	};

	# Exceptions: a list of dates for each kind. A range is 'from..to', both ends included.
	days = {
		off = [
			'2026-11-03', # a public holiday
			'2026-12-29..2027-01-03', # New Year holidays
		];
		work = [
			'2026-11-08', # working on a Sunday
		];
		light = [
			'2026-11-20',
		];
	};

	# Exceptions that have passed. The app moves them here from days; you need not write them.
	past_days = {
		off = [
			'2026-10-13',
		];
	};
}
```

The format is [setay](https://github.com/aki-kuramoto/setay-go). A `#` starts a comment that runs to the end of the line.

## Entries

- **`kinds`**: the names of the kinds of days and their weights.
  - A name starts with a letter or `_`, and has only letters, digits, `_` and `-`.
  - A weight is a number of 0 or more: how much you expect to use Claude on such a day. Only the ratios matter, so "1 for days you use it, 0 for days you don't" works, and so does "1 for weekdays, 4 for weekends" if you think in hours.
  - The names carry no meaning such as "holiday" or "working day". If you use Claude only at weekends, write something like `weekend = 1; weekday = 0;`.
- **`weekdays`**: the kind of each day of the week. Write all seven of `mon`, `tue`, `wed`, `thu`, `fri`, `sat` and `sun`, each with a name from `kinds`.
- **`days`**: exceptions. For each kind, a list of dates (`'2026-11-03'`) or ranges (`'2026-12-29..2027-01-03'`, both ends included). They take precedence over `weekdays`.
- **`past_days`**: exceptions that have passed, in the same form as `days`.

## What the app does

- **Moves past exceptions**: when it starts and when the date changes, the app moves every exception in `days` whose last day is yesterday or earlier to `past_days`, under the same kind. They are moved rather than deleted, so that "forgot to write it" and "it has passed" can be told apart. A range is moved once its last day has passed.
  - An exception written on a line of its own is moved with the comment at the end of that line.
  - When some entries of a list written on one line are moved, the rest of that list is written back on one line (comments inside that list are lost).
  - Other lines, comments and formatting are left as they are.
  - If the file changes while entries are being moved, the app does nothing that time and tries again later.
- **The triangle above** ("by usage days"): the weekly period (from seven days before the reset time to the reset time) is split into days, and their weights are added up. The share of that total that has already passed is where the triangle goes. It depends on the days that have passed too, so after light days (weekends, say) have passed, it does not match the vertical line (the time elapsed) even if only heavy days remain. Both reach 100% together at the reset time.
- **The triangle below** ("by the days ahead"): the time left until the reset is converted by the weights. An hour of the heaviest kind of day counts as an hour; an hour of a day weighing a ninth of that counts as a ninth of an hour. The triangle goes at "1 − (converted time left ÷ length of the period)".
  - If all the days left are of the heaviest kind, it is at the same place as the vertical line.
  - If light days are among those left, it is to the right of the line: "light days are coming, so using a little more by now is fine".
  - The weights of the days that have passed do not matter.
  - When every kind weighs 0, it is not shown.
- Days start at midnight in the machine's time zone. The first and last days of the period count by the share of their hours inside it, and the weight of a day is taken as spread evenly over its hours.
- When the weights of the whole period add up to 0 (no use at all in the period), the triangle above is not shown.
- The five-hour limit has neither triangle: it ends within a day, so the weights of days mean nothing there.
- Days in `past_days` are used too, since the weekly period includes days that have passed.

## Mistakes in the file

In these cases, the app does not use the file at all: it shows no triangles and says why in red in the window. The operation log records it as `days_error`.

- An unknown entry (such as `holidays`)
- `kinds` is empty, or a weight is negative
- `weekdays` misses a day or has an unknown one
- A kind that is not in `kinds` is used
- A date that cannot be read, a range that ends before it starts, or a range longer than two years
- The same day under two kinds (including one in `days` and one in `past_days`)

---

# 日の設定 (days.setay) (日本語)

`days.setay` は、claude-usage-visualizer に、日ごとにどれくらい Claude を使うかを伝えるファイルです。これをもとに、週の枠のバーに 2 つの小さな三角を出します。

- バーの**上**の下向きの三角: Claude を使う日だけで数えると、リセットまでの期間のうちどれだけ過ぎたか。
- バーの**下**の上向きの三角: これから先の日の重みから見て、今どこまで来ているとみなせるか。

曜日ごとにどれくらい使うかと、祝日や休暇などの例外の日を書きます。自分で書いても、この文書をエージェントに渡して書いてもらってもかまいません。

## 置き場所

ホームディレクトリの下の `.app-data/claude-usage-visualizer/days.setay` です (macOS なら `~/.app-data/claude-usage-visualizer/days.setay`、Windows なら `%USERPROFILE%\.app-data\claude-usage-visualizer\days.setay`)。

起動したときにファイルがなければ、アプリが書きます。内容は、平日 (`weekday`) の重みが 9、土日 (`weekend`) の重みが 1 で、例外の日はなし (書き方の例をコメントで入れてある) です。既にあるファイルは決して上書きしません。消すと、次に起動したときにまた書かれます。

アプリは動いている間もファイルの変更を拾うので、起動し直す必要はありません (画面への反映は最大 30 秒後)。動いている間にファイルを消した場合は、三角が出なくなるだけです。

## 例

```
{
	# 日の種類と、その重み。名前は自由に付けられる。
	kinds = {
		work = 1;
		off = 0;
		light = 0.3;
	};

	# 曜日ごとの種類。7 つの曜日すべてを書く。
	weekdays = {
		mon = 'work'; tue = 'work'; wed = 'work'; thu = 'work'; fri = 'work';
		sat = 'off'; sun = 'off';
	};

	# 例外の日。種類ごとの一覧。範囲は 'から..まで' (両端を含む)。
	days = {
		off = [
			'2026-11-03', # 文化の日
			'2026-12-29..2027-01-03', # 年末年始
		];
		work = [
			'2026-11-08', # 日曜だけど仕事
		];
		light = [
			'2026-11-20',
		];
	};

	# 過ぎた例外。アプリが days から移すので、自分で書かなくてよい。
	past_days = {
		off = [
			'2026-10-13',
		];
	};
}
```

形式は [setay](https://github.com/aki-kuramoto/setay-go) です。`#` から行の終わりまではコメントです。

## 各項目

- **`kinds`**: 日の種類の名前と重みです。
  - 名前は英字か `_` で始まり、英字、数字、`_`、`-` だけを使います。
  - 重みは 0 以上の数で、その日にどれくらい使うかの見込みを表します。比だけが意味を持つので、「使う日 1、使わない日 0」でも、時間数のつもりで「平日 1、週末 4」でもかまいません。
  - 名前に「休日」「稼働日」のような意味は決まっていません。土日だけ使う人は `weekend = 1; weekday = 0;` のように書きます。
- **`weekdays`**: 曜日ごとの種類です。`mon`、`tue`、`wed`、`thu`、`fri`、`sat`、`sun` の 7 つすべてを書き、値には `kinds` にある名前を書きます。
- **`days`**: 例外の日です。種類の名前ごとに、日付 (`'2026-11-03'`) か範囲 (`'2026-12-29..2027-01-03'`、両端を含む) の一覧を書きます。`weekdays` より優先します。
- **`past_days`**: 過ぎた例外です。`days` と同じ形です。

## アプリがすること

- **過ぎた例外を移す**: 起動したときと日付が変わったときに、`days` の中で最後の日が昨日以前になった例外を、同じ種類の名前で `past_days` に移します。消さずに移すのは、「書き忘れた」のか「過ぎただけ」なのかを見分けられるようにするためです。範囲は、最後の日が過ぎてから移します。
  - 1 行に 1 つずつ書いた例外は、その行の終わりのコメントも一緒に移します。
  - 1 行に複数並べた一覧から移すときは、残りをその一覧の 1 行に書き直します (一覧の中のコメントは消えます)。
  - それ以外の行、コメント、書き方は変えません。
  - 移している最中にファイルが書き換えられていたら、その回は何もせず、あとでやり直します。
- **上の三角 (「使う日で」)**: 週の枠の期間 (リセット時刻の 7 日前からリセット時刻まで) を日ごとに区切り、各日の重みを足し合わせます。その合計のうち、今までに過ぎた分の割合が三角の位置です。過ぎた日の重みにも左右されるので、軽い日 (例えば土日) が過ぎたあとは、残りが重い日だけでも、縦線 (時間の経過) とは一致しません。リセットの時刻に両方が 100% で重なります。
- **下の三角 (「残りの日で」)**: 今からリセットまでの残り時間を、日の重みで換算し直します。いちばん重い種類の日の 1 時間を 1 時間、重みがその 9 分の 1 の日の 1 時間を 9 分の 1 時間と数えます。三角の位置は「1 − (換算した残りの時間 ÷ 期間の長さ)」です。
  - 残りがすべていちばん重い種類の日なら、縦線と同じ位置に来ます。
  - 残りに軽い日が含まれていると、縦線より右に来ます。「この先は軽い日が控えているので、今の時点でもう少し使っていても大丈夫」という意味です。
  - 過ぎた日の重みは関係しません。
  - どの種類の重みも 0 のときは出しません。
- 1 日の区切りはマシンのタイムゾーンの 0 時です。期間の最初と最後の日は、期間に入っている時間の割合で数え、1 日の中では重みが時間に均等に配られているとみなします。
- 期間の重みの合計が 0 のとき (期間中ずっと使わない設定) は、上の三角を出しません。
- 5 時間枠には、どちらの三角も出しません。1 日の中で終わる枠なので、日の重みが意味を持たないためです。
- `past_days` の日も計算に使います。週の枠の期間には、過ぎた日も含まれるためです。

## 書き間違いの扱い

次のようなときは、ファイル全体を使わず、三角を出さずに、画面に赤い文字で理由を出します。動作の記録にも `days_error` として残します。

- 知らない項目名がある (`holidays` など)
- `kinds` が空、または重みが負の数
- `weekdays` に足りない曜日や知らない曜日がある
- `kinds` にない種類の名前を使っている
- 日付として読めない、範囲の終わりが始まりより前、範囲が 2 年より長い
- 同じ日が 2 つの種類に入っている (`days` と `past_days` をまたぐ場合も含む)
