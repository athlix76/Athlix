import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/widgets/error_view.dart';
import '../application/follow_controller.dart';
import '../data/follow_api.dart';

/// Follow / Following toggle for one user. Keeps its own state after the first tap;
/// pass a new [key] if the parent reloads [initialFollowing].
class FollowButton extends ConsumerStatefulWidget {
  const FollowButton({super.key, required this.userId, required this.initialFollowing, this.onChanged, this.dense = false});

  final int userId;
  final bool initialFollowing;
  final ValueChanged<FollowState>? onChanged;
  final bool dense;

  @override
  ConsumerState<FollowButton> createState() => _FollowButtonState();
}

class _FollowButtonState extends ConsumerState<FollowButton> {
  late bool _following = widget.initialFollowing;
  bool _busy = false;

  Future<void> _toggle() async {
    setState(() => _busy = true);
    try {
      final s = await ref.read(followControllerProvider).setFollowing(widget.userId, !_following);
      if (!mounted) return;
      setState(() => _following = s.following);
      widget.onChanged?.call(s);
    } catch (e) {
      if (mounted) showErrorSnack(context, e);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final child = _busy
        ? const SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2))
        : Text(_following ? 'Following' : 'Follow');
    final style = widget.dense ? const ButtonStyle(visualDensity: VisualDensity.compact) : null;
    return _following
        ? OutlinedButton(style: style, onPressed: _busy ? null : _toggle, child: child)
        : FilledButton(style: style, onPressed: _busy ? null : _toggle, child: child);
  }
}
