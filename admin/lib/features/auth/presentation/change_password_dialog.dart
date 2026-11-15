import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/api_error.dart';
import '../../../core/widgets/api_error_view.dart';
import '../application/session_controller.dart';

/// Change the signed-in admin's own password (`POST /admin/me/password`).
/// Needs the current password and an authenticator code. The strength rules
/// live on the server (`newPassword` field error). Every other session of the
/// admin is signed out; this one continues on the new tokens.
Future<void> showChangePasswordDialog(BuildContext context) async {
  final changed = await showDialog<bool>(
    context: context,
    barrierDismissible: false,
    builder: (_) => const _ChangePasswordDialog(),
  );
  if (changed == true && context.mounted) {
    ScaffoldMessenger.of(context).showSnackBar(
      const SnackBar(content: Text('Password changed. Other sessions were signed out.')),
    );
  }
}

class _ChangePasswordDialog extends ConsumerStatefulWidget {
  const _ChangePasswordDialog();

  @override
  ConsumerState<_ChangePasswordDialog> createState() => _ChangePasswordDialogState();
}

class _ChangePasswordDialogState extends ConsumerState<_ChangePasswordDialog> {
  final _form = GlobalKey<FormState>();
  final _current = TextEditingController();
  final _next = TextEditingController();
  final _repeat = TextEditingController();
  final _totp = TextEditingController();
  bool _busy = false;
  ApiError? _error;

  @override
  void dispose() {
    _current.dispose();
    _next.dispose();
    _repeat.dispose();
    _totp.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    if (!(_form.currentState?.validate() ?? false)) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await ref.read(sessionProvider).changePassword(
            currentPassword: _current.text,
            newPassword: _next.text,
            totpCode: _totp.text,
          );
      if (mounted) Navigator.of(context).pop(true);
    } on ApiError catch (e) {
      _totp.clear(); // a code is single-use
      if (mounted) setState(() => _error = e);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  String? _fieldError(String key) => _error?.fields[key];

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: const Text('Change password'),
      content: SizedBox(
        width: 380,
        child: Form(
          key: _form,
          child: AutofillGroup(
            child: Column(mainAxisSize: MainAxisSize.min, children: [
              if (_error != null) ...[ApiErrorBanner(error: _error!), const SizedBox(height: 12)],
              TextFormField(
                controller: _current,
                obscureText: true,
                autofillHints: const [AutofillHints.password],
                decoration: InputDecoration(labelText: 'Current password', errorText: _fieldError('currentPassword')),
                validator: (v) => (v == null || v.isEmpty) ? 'Enter your current password' : null,
              ),
              TextFormField(
                controller: _next,
                obscureText: true,
                autofillHints: const [AutofillHints.newPassword],
                decoration: InputDecoration(labelText: 'New password', errorText: _fieldError('newPassword')),
                validator: (v) => (v == null || v.isEmpty) ? 'Enter a new password' : null,
              ),
              TextFormField(
                controller: _repeat,
                obscureText: true,
                autofillHints: const [AutofillHints.newPassword],
                decoration: const InputDecoration(labelText: 'Repeat new password'),
                validator: (v) => v != _next.text ? 'The passwords do not match' : null,
              ),
              TextFormField(
                controller: _totp,
                keyboardType: TextInputType.number,
                autofillHints: const [AutofillHints.oneTimeCode],
                decoration: InputDecoration(labelText: 'Authenticator code', errorText: _fieldError('totpCode')),
                validator: (v) => (v == null || v.trim().isEmpty) ? 'Enter the code from your app' : null,
                onFieldSubmitted: (_) => _submit(),
              ),
            ]),
          ),
        ),
      ),
      actions: [
        TextButton(onPressed: _busy ? null : () => Navigator.of(context).pop(false), child: const Text('Cancel')),
        FilledButton(onPressed: _busy ? null : _submit, child: Text(_busy ? 'Saving…' : 'Change password')),
      ],
    );
  }
}
