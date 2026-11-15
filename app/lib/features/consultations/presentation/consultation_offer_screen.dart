import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/api/api_exception.dart';
import '../../../core/widgets/busy_button.dart';
import '../../../core/widgets/error_view.dart';
import '../application/consultation_controller.dart';
import '../data/consultation_models.dart';

/// `/consultations/offer`: a coach turns their consultation offer on or off
/// (`PUT /me/consultation`). The fee note is free text: players pay the coach
/// directly, Athlix never collects it. Listing may need a plan with the
/// `consultation_listing` benefit (403 `PLAN_REQUIRED` offers premium).
class ConsultationOfferScreen extends ConsumerWidget {
  const ConsultationOfferScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final offer = ref.watch(myConsultationOfferProvider);
    return Scaffold(
      appBar: AppBar(title: const Text('My consultation offer')),
      body: offer.when(
        skipLoadingOnRefresh: true,
        loading: () => const Center(child: CircularProgressIndicator()),
        // No offer yet → empty form.
        error: (e, _) => e is ApiException && e.code == ApiErrorCode.notFound
            ? const _OfferForm(initial: null)
            : ErrorView(error: e, onRetry: () => ref.invalidate(myConsultationOfferProvider)),
        data: (o) => _OfferForm(initial: o),
      ),
    );
  }
}

class _OfferForm extends ConsumerStatefulWidget {
  const _OfferForm({required this.initial});

  final ConsultationOffer? initial;

  @override
  ConsumerState<_OfferForm> createState() => _OfferFormState();
}

class _OfferFormState extends ConsumerState<_OfferForm> {
  late bool _offered = widget.initial?.offered ?? true;
  late final _description = TextEditingController(text: widget.initial?.description ?? '');
  late final _feeNote = TextEditingController(text: widget.initial?.feeNote ?? '');
  late final Set<String> _modes = {...?widget.initial?.modes};

  @override
  void dispose() {
    _description.dispose();
    _feeNote.dispose();
    super.dispose();
  }

  Future<void> _save() async {
    if (_offered && _modes.isEmpty) {
      showInfoSnack(context, 'Choose at least one mode.');
      return;
    }
    await ref.read(consultationControllerProvider).saveOffer(ConsultationOfferInput(
          offered: _offered,
          description: _description.text.trim(),
          feeNote: _feeNote.text.trim(),
          modes: ConsultationMode.values.where(_modes.contains).toList(),
        ));
    if (!mounted) return;
    showInfoSnack(context, _offered ? 'Your offer is listed.' : 'Your offer is hidden.');
    context.pop();
  }

  @override
  Widget build(BuildContext context) {
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        SwitchListTile(
          contentPadding: EdgeInsets.zero,
          title: const Text('Offer consultations'),
          subtitle: const Text('Players can see your offer and send you a request.'),
          value: _offered,
          onChanged: (v) => setState(() => _offered = v),
        ),
        const SizedBox(height: 12),
        TextField(
          controller: _description,
          minLines: 3,
          maxLines: 8,
          decoration: const InputDecoration(
            labelText: 'What you offer',
            hintText: 'e.g. batting technique review, fitness plans',
          ),
        ),
        const SizedBox(height: 12),
        TextField(
          controller: _feeNote,
          decoration: const InputDecoration(
            labelText: 'Fee note (optional)',
            hintText: 'e.g. per-session fee, paid to me directly',
            helperText: 'Athlix does not collect consultation fees. Players pay you directly.',
            helperMaxLines: 2,
          ),
        ),
        const SizedBox(height: 16),
        Text('Modes', style: Theme.of(context).textTheme.titleSmall),
        Wrap(
          spacing: 8,
          children: [
            for (final m in ConsultationMode.values)
              FilterChip(
                label: Text(ConsultationMode.labels[m]!),
                selected: _modes.contains(m),
                onSelected: (on) => setState(() => on ? _modes.add(m) : _modes.remove(m)),
              ),
          ],
        ),
        const SizedBox(height: 24),
        BusyButton(label: 'Save', onPressed: _save),
      ],
    );
  }
}
