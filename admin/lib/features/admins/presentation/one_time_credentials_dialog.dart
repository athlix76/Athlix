import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:qr_flutter/qr_flutter.dart';

import '../data/admins_api.dart';

/// Shows a temporary password and the TOTP enrolment QR once. Nothing here is
/// stored or logged; closing the dialog drops the values.
Future<void> showOneTimeCredentials(BuildContext context, {required String adminName, required OneTimeCredentials creds}) =>
    showDialog<void>(
      context: context,
      barrierDismissible: false,
      builder: (ctx) => AlertDialog(
        title: Text('Credentials for $adminName'),
        content: SizedBox(
          width: 420,
          child: SingleChildScrollView(
            child: Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.start, children: [
              Text(
                'Shown only once. Hand them to the admin over a private channel (in person or a call), never by email or chat. '
                'They scan the QR in an authenticator app before their first sign-in.',
                style: TextStyle(color: Theme.of(ctx).colorScheme.error),
              ),
              const SizedBox(height: 16),
              const Text('Temporary password'),
              Row(children: [
                Expanded(child: SelectableText(creds.temporaryPassword, style: const TextStyle(fontFamily: 'monospace', fontSize: 18))),
                IconButton(
                  tooltip: 'Copy',
                  icon: const Icon(Icons.copy),
                  onPressed: () => Clipboard.setData(ClipboardData(text: creds.temporaryPassword)),
                ),
              ]),
              const SizedBox(height: 16),
              const Text('Authenticator (TOTP)'),
              const SizedBox(height: 8),
              Center(
                child: ColoredBox(
                  color: Colors.white,
                  child: Padding(
                    padding: const EdgeInsets.all(8),
                    child: QrImageView(data: creds.totpUri, size: 220),
                  ),
                ),
              ),
              ExpansionTile(
                tilePadding: EdgeInsets.zero,
                title: const Text("Can't scan? Show the setup URI"),
                children: [SelectableText(creds.totpUri, style: const TextStyle(fontFamily: 'monospace'))],
              ),
            ]),
          ),
        ),
        actions: [FilledButton(onPressed: () => Navigator.of(ctx).pop(), child: const Text('I have handed these over'))],
      ),
    );
