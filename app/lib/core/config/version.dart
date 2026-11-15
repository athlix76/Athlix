/// Compares dotted versions (`1.2.10` > `1.2.9`). Build suffixes (`+5`, `-beta`)
/// are ignored. Returns negative / 0 / positive like [Comparable.compareTo].
int compareVersions(String a, String b) {
  List<int> parts(String v) => v
      .split(RegExp(r'[+\-]'))
      .first
      .split('.')
      .map((p) => int.tryParse(p.trim()) ?? 0)
      .toList();
  final pa = parts(a);
  final pb = parts(b);
  final n = pa.length > pb.length ? pa.length : pb.length;
  for (var i = 0; i < n; i++) {
    final x = i < pa.length ? pa[i] : 0;
    final y = i < pb.length ? pb[i] : 0;
    if (x != y) return x.compareTo(y);
  }
  return 0;
}

/// True when [current] is below [minimum]. An unknown minimum never blocks.
bool needsForceUpdate(String current, String? minimum) {
  if (minimum == null || minimum.trim().isEmpty) return false;
  return compareVersions(current, minimum) < 0;
}
