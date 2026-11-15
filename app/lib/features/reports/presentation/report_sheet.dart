import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/widgets/busy_button.dart';
import '../../../core/widgets/error_view.dart';
import '../data/report_api.dart';

export '../data/report_api.dart' show ReportTarget, ReportReason;

/// Bottom sheet to report any target. Use [ReportTarget] for [targetType].
Future<void> showReportSheet(BuildContext context, {required String targetType, required int targetId}) async {
  final sent = await showModalBottomSheet<bool>(
    context: context,
    isScrollControlled: true,
    showDragHandle: true,
    builder: (_) => _ReportSheet(targetType: targetType, targetId: targetId),
  );
  if (sent == true && context.mounted) {
    showInfoSnack(context, 'Thanks. Our team will review this report.');
  }
}

class _ReportSheet extends ConsumerStatefulWidget {
  const _ReportSheet({required this.targetType, required this.targetId});

  final String targetType;
  final int targetId;

  @override
  ConsumerState<_ReportSheet> createState() => _ReportSheetState();
}

class _ReportSheetState extends ConsumerState<_ReportSheet> {
  String? _reason;
  final _details = TextEditingController();

  @override
  void dispose() {
    _details.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    final reason = _reason;
    if (reason == null) return;
    await ref.read(reportApiProvider).create(
          targetType: widget.targetType,
          targetId: widget.targetId,
          reason: reason,
          details: _details.text.trim(),
        );
    if (mounted) Navigator.of(context).pop(true);
  }

  @override
  Widget build(BuildContext context) {
    final bottom = MediaQuery.of(context).viewInsets.bottom;
    return Padding(
      padding: EdgeInsets.fromLTRB(16, 0, 16, 16 + bottom),
      child: SingleChildScrollView(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text('Report', style: Theme.of(context).textTheme.titleLarge),
            const SizedBox(height: 8),
            for (final r in ReportReason.all)
              RadioListTile<String>(
                value: r,
                groupValue: _reason,
                title: Text(ReportReason.labels[r] ?? r),
                contentPadding: EdgeInsets.zero,
                onChanged: (v) => setState(() => _reason = v),
              ),
            const SizedBox(height: 8),
            TextField(
              controller: _details,
              maxLength: 1000,
              minLines: 2,
              maxLines: 5,
              decoration: const InputDecoration(labelText: 'Details (optional)'),
            ),
            const SizedBox(height: 8),
            BusyButton(label: 'Send report', onPressed: _reason == null ? null : _submit),
          ],
        ),
      ),
    );
  }
}
