import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/config/config_providers.dart';
import '../../../core/router/routes.dart';
import '../../../core/session/session_controller.dart';
import '../../../core/widgets/async_value_view.dart';
import '../../../core/widgets/busy_button.dart';
import '../../../core/widgets/error_view.dart';
import '../../../core/widgets/section.dart';
import '../../../core/widgets/sport_picker.dart';
import '../../../core/widgets/user_tile.dart';
import '../application/consultation_controller.dart';
import '../data/consultation_models.dart';

/// `/coaches/:id/consultation`: a coach's offer and a request form.
/// The fee is paid directly to the coach; Athlix only lists the offer.
class CoachConsultationScreen extends ConsumerWidget {
  const CoachConsultationScreen({super.key, required this.coachId});

  final int coachId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final offer = ref.watch(consultationOfferProvider(coachId));
    final isMe = ref.watch(currentUserIdProvider) == coachId;
    return Scaffold(
      appBar: AppBar(title: const Text('Consultation')),
      body: AsyncValueView<ConsultationOffer>(
        value: offer,
        onRetry: () => ref.invalidate(consultationOfferProvider(coachId)),
        data: (o) => ListView(
          children: [
            if (o.coach != null) UserTile(user: o.coach!),
            if (!o.offered)
              const Padding(
                padding: EdgeInsets.all(16),
                child: Text('Consultations are not offered right now.'),
              ),
            if ((o.description ?? '').isNotEmpty) ...[
              const SectionHeader('About'),
              Padding(
                padding: const EdgeInsets.symmetric(horizontal: 16),
                child: Text(o.description!),
              ),
            ],
            InfoRow(
              label: 'Modes',
              value: o.modes.map((m) => ConsultationMode.labels[m] ?? m).join(', '),
              icon: Icons.videocam_outlined,
            ),
            InfoRow(label: 'Fee', value: o.feeNote ?? '', icon: Icons.payments_outlined),
            const Padding(
              padding: EdgeInsets.symmetric(horizontal: 16, vertical: 4),
              child: Text(
                'Any fee is agreed with and paid directly to the coach. Athlix does not collect it.',
              ),
            ),
            if (isMe)
              Padding(
                padding: const EdgeInsets.all(16),
                child: OutlinedButton(
                  onPressed: () => context.push(Routes.consultationOffer),
                  child: const Text('Edit my offer'),
                ),
              )
            else if (o.offered && o.canRequest)
              _RequestForm(coachId: coachId, modes: o.modes)
            else if (o.offered)
              const Padding(
                padding: EdgeInsets.all(16),
                child: Text('You cannot send a request right now. You may already have an open '
                    'request with this coach, or reached the limit of open requests.'),
              ),
          ],
        ),
      ),
    );
  }
}

class _RequestForm extends ConsumerStatefulWidget {
  const _RequestForm({required this.coachId, required this.modes});

  final int coachId;
  final List<String> modes;

  @override
  ConsumerState<_RequestForm> createState() => _RequestFormState();
}

class _RequestFormState extends ConsumerState<_RequestForm> {
  final _message = TextEditingController();
  late String? _mode = widget.modes.isEmpty ? null : widget.modes.first;

  @override
  void dispose() {
    _message.dispose();
    super.dispose();
  }

  Future<void> _send() async {
    final text = _message.text.trim();
    final mode = _mode;
    if (text.isEmpty || mode == null) {
      showInfoSnack(context, 'Write a message and choose a mode.');
      return;
    }
    await ref.read(consultationControllerProvider).request(widget.coachId, text, mode);
    if (!mounted) return;
    showInfoSnack(context, 'Request sent.');
    context.pushReplacement(Routes.consultations);
  }

  @override
  Widget build(BuildContext context) {
    final maxOpen = ref.watch(appConfigProvider).maxOpenConsultationRequests;
    final modes = widget.modes.isEmpty ? ConsultationMode.values : widget.modes;
    return Padding(
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text('Request a consultation', style: Theme.of(context).textTheme.titleMedium),
          const SizedBox(height: 12),
          TextField(
            controller: _message,
            minLines: 3,
            maxLines: 6,
            decoration: const InputDecoration(
              labelText: 'Message',
              hintText: 'What would you like help with?',
            ),
          ),
          const SizedBox(height: 12),
          EnumDropdown(
            label: 'Preferred mode',
            values: modes,
            labels: ConsultationMode.labels,
            value: _mode,
            onChanged: (v) => setState(() => _mode = v),
          ),
          if (maxOpen != null)
            Padding(
              padding: const EdgeInsets.only(top: 8),
              child: Text('You can have up to $maxOpen open requests at a time.',
                  style: Theme.of(context).textTheme.bodySmall),
            ),
          const SizedBox(height: 16),
          BusyButton(label: 'Send request', onPressed: _send),
        ],
      ),
    );
  }
}
