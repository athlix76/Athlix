/// A minimal line diff (longest common subsequence) for "review before save".
library;

enum DiffOp { same, removed, added }

class DiffLine {
  const DiffLine(this.op, this.text);

  final DiffOp op;
  final String text;

  @override
  bool operator ==(Object other) => other is DiffLine && other.op == op && other.text == text;

  @override
  int get hashCode => Object.hash(op, text);

  @override
  String toString() => '${op.name}:$text';
}

List<DiffLine> lineDiff(String before, String after) {
  final a = before.split('\n');
  final b = after.split('\n');
  final n = a.length;
  final m = b.length;
  final lcs = List.generate(n + 1, (_) => List<int>.filled(m + 1, 0));
  for (var i = n - 1; i >= 0; i--) {
    for (var j = m - 1; j >= 0; j--) {
      lcs[i][j] = a[i] == b[j] ? lcs[i + 1][j + 1] + 1 : (lcs[i + 1][j] >= lcs[i][j + 1] ? lcs[i + 1][j] : lcs[i][j + 1]);
    }
  }
  final out = <DiffLine>[];
  var i = 0;
  var j = 0;
  while (i < n && j < m) {
    if (a[i] == b[j]) {
      out.add(DiffLine(DiffOp.same, a[i]));
      i++;
      j++;
    } else if (lcs[i + 1][j] >= lcs[i][j + 1]) {
      out.add(DiffLine(DiffOp.removed, a[i]));
      i++;
    } else {
      out.add(DiffLine(DiffOp.added, b[j]));
      j++;
    }
  }
  while (i < n) {
    out.add(DiffLine(DiffOp.removed, a[i++]));
  }
  while (j < m) {
    out.add(DiffLine(DiffOp.added, b[j++]));
  }
  return out;
}
