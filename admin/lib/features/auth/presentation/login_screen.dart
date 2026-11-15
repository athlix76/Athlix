import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/widgets/api_error_view.dart';
import '../application/session_controller.dart';

/// Email + password + TOTP code, in one request (`POST /admin/auth/login`).
class LoginScreen extends ConsumerStatefulWidget {
  const LoginScreen({super.key});

  @override
  ConsumerState<LoginScreen> createState() => _LoginScreenState();
}

class _LoginScreenState extends ConsumerState<LoginScreen> {
  final _form = GlobalKey<FormState>();
  final _email = TextEditingController();
  final _password = TextEditingController();
  final _totp = TextEditingController();
  bool _busy = false;
  Object? _error;
  String? _notice;

  @override
  void initState() {
    super.initState();
    _notice = ref.read(sessionProvider).takeNotice();
  }

  @override
  void dispose() {
    _email.dispose();
    _password.dispose();
    _totp.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    if (!_form.currentState!.validate()) return;
    setState(() {
      _busy = true;
      _error = null;
      _notice = null;
    });
    try {
      await ref.read(sessionProvider).login(email: _email.text, password: _password.text, totpCode: _totp.text);
      // The router redirects away from /login once signed in.
    } catch (e) {
      if (mounted) {
        setState(() {
          _error = e;
          _busy = false;
        });
      }
    } finally {
      _password.clear();
      _totp.clear();
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: Center(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(24),
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 400),
            child: Card(
              child: Padding(
                padding: const EdgeInsets.all(24),
                child: Form(
                  key: _form,
                  child: AutofillGroup(
                    child: Column(
                      mainAxisSize: MainAxisSize.min,
                      crossAxisAlignment: CrossAxisAlignment.stretch,
                      children: [
                        Text('Athlix admin', style: Theme.of(context).textTheme.headlineSmall),
                        const SizedBox(height: 4),
                        const Text('Sign in with your email, password and authenticator code.'),
                        const SizedBox(height: 20),
                        if (_notice != null) ...[
                          Text(_notice!, style: TextStyle(color: Theme.of(context).colorScheme.primary)),
                          const SizedBox(height: 12),
                        ],
                        TextFormField(
                          controller: _email,
                          autofillHints: const [AutofillHints.username, AutofillHints.email],
                          keyboardType: TextInputType.emailAddress,
                          decoration: const InputDecoration(labelText: 'Email', border: OutlineInputBorder()),
                          validator: (v) => (v == null || !v.contains('@')) ? 'Enter your email' : null,
                        ),
                        const SizedBox(height: 12),
                        TextFormField(
                          controller: _password,
                          obscureText: true,
                          autofillHints: const [AutofillHints.password],
                          decoration: const InputDecoration(labelText: 'Password', border: OutlineInputBorder()),
                          validator: (v) => (v == null || v.isEmpty) ? 'Enter your password' : null,
                        ),
                        const SizedBox(height: 12),
                        TextFormField(
                          controller: _totp,
                          autofillHints: const [AutofillHints.oneTimeCode],
                          keyboardType: TextInputType.number,
                          inputFormatters: [FilteringTextInputFormatter.digitsOnly, LengthLimitingTextInputFormatter(8)],
                          decoration: const InputDecoration(labelText: 'Authenticator code', border: OutlineInputBorder()),
                          validator: (v) => (v == null || v.length < 6) ? 'Enter the 6-digit code' : null,
                          onFieldSubmitted: (_) => _submit(),
                        ),
                        const SizedBox(height: 16),
                        if (_error != null) ...[ApiErrorBanner(error: _error!), const SizedBox(height: 12)],
                        FilledButton(
                          onPressed: _busy ? null : _submit,
                          child: _busy
                              ? const SizedBox(width: 18, height: 18, child: CircularProgressIndicator(strokeWidth: 2))
                              : const Text('Sign in'),
                        ),
                        const SizedBox(height: 12),
                        Text(
                          'For security the session lives only in this tab: reloading or closing it signs you out.',
                          style: Theme.of(context).textTheme.bodySmall,
                        ),
                      ],
                    ),
                  ),
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}
