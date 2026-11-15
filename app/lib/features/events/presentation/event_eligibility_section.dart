import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/config/config_providers.dart';
import '../../../core/widgets/section.dart';
import '../../../core/widgets/sport_picker.dart';
import '../../location_identity/data/eligibility_models.dart';
import '../../location_identity/presentation/geo_pickers.dart';
import '../application/event_providers.dart';
import '../data/event_models.dart';

const _typeLabels = {
  EligibilityType.open: 'Open to everyone',
  EligibilityType.native: 'Native players only',
  EligibilityType.residence: 'Residents only',
  EligibilityType.either: 'Native or resident',
};

/// Organiser form "Eligibility" section. Edits [input] in place; the server
/// validates and renders the preview line. Locked once published.
class EventEligibilitySection extends ConsumerStatefulWidget {
  const EventEligibilitySection({
    super.key,
    required this.input,
    required this.locked,
    required this.isNew,
  });

  final EventInput input;
  final bool locked;

  /// New events take the config default type for the category.
  final bool isNew;

  @override
  ConsumerState<EventEligibilitySection> createState() => _EventEligibilitySectionState();
}

class _EventEligibilitySectionState extends ConsumerState<EventEligibilitySection> {
  Timer? _debounce;
  EligibilityPreviewKey? _key;

  EventInput get _in => widget.input;
  EligibilityInput get _rule => _in.eligibility;

  @override
  void initState() {
    super.initState();
    if (widget.isNew) _applyDefaultType();
    _schedulePreview();
  }

  @override
  void dispose() {
    _debounce?.cancel();
    super.dispose();
  }

  /// `event.eligibility_default_government` / `_private`.
  void _applyDefaultType() {
    final config = ref.read(appConfigProvider);
    final suggested = _in.category == EventCategory.government
        ? config.eventEligibilityDefaultGovernment
        : config.eventEligibilityDefaultPrivate;
    if (suggested != null && EligibilityType.all.contains(suggested)) _setType(suggested);
  }

  void _setType(String type) {
    _rule.type = type;
    if (type == EligibilityType.open) {
      _rule
        ..scope = null
        ..districtId = null;
    } else {
      _rule.scope ??= EligibilityScope.district;
    }
  }

  void _update(VoidCallback change) {
    setState(change);
    _schedulePreview();
  }

  void _schedulePreview() {
    if (widget.locked) return;
    _debounce?.cancel();
    _debounce = Timer(const Duration(milliseconds: 400), () {
      final j = _rule.toJson();
      final key = (
        category: _in.category,
        type: _rule.type,
        scope: j['scope'] as String?,
        districtId: j['districtId'] as int?,
        stateId: j['stateId'] as int?,
        requireVerified: _rule.requireVerified,
      );
      if (mounted && key != _key) setState(() => _key = key);
    });
  }

  @override
  Widget build(BuildContext context) {
    final config = ref.watch(appConfigProvider);
    if (!config.locationEnabled) return const SizedBox.shrink();
    final theme = Theme.of(context);
    final open = _rule.type == EligibilityType.open;
    final key = _key;

    final fields = Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text('Organised by', style: theme.textTheme.labelLarge),
        const SizedBox(height: 4),
        SegmentedButton<String>(
          segments: const [
            ButtonSegment(value: EventCategory.private, label: Text('Private')),
            ButtonSegment(value: EventCategory.government, label: Text('Government')),
          ],
          selected: {_in.category},
          onSelectionChanged: (v) => _update(() {
            _in.category = v.first;
            _applyDefaultType();
          }),
        ),
        const SizedBox(height: 12),
        EnumDropdown(
          label: 'Who can join',
          values: EligibilityType.all,
          value: _rule.type,
          labels: _typeLabels,
          onChanged: (v) => _update(() => _setType(v ?? EligibilityType.open)),
        ),
        if (!open) ...[
          const SizedBox(height: 12),
          SegmentedButton<String>(
            segments: const [
              ButtonSegment(value: EligibilityScope.district, label: Text('District')),
              ButtonSegment(value: EligibilityScope.state, label: Text('State')),
            ],
            selected: {_rule.scope ?? EligibilityScope.district},
            onSelectionChanged: (v) => _update(() => _rule.scope = v.first),
          ),
          const SizedBox(height: 12),
          StateDropdown(
            value: _rule.stateId,
            onChanged: (v) => _update(() {
              _rule
                ..stateId = v
                ..districtId = null;
            }),
          ),
          if (_rule.scope != EligibilityScope.state) ...[
            const SizedBox(height: 12),
            DistrictDropdown(
              stateId: _rule.stateId,
              value: _rule.districtId,
              onChanged: (v) => _update(() => _rule.districtId = v),
            ),
          ],
          SwitchListTile(
            contentPadding: EdgeInsets.zero,
            value: _rule.requireVerified,
            onChanged: (v) => _update(() => _rule.requireVerified = v),
            title: const Text('Require verified identity'),
            subtitle: const Text('When off, players whose proof is still in review can register too.'),
          ),
        ],
        const SizedBox(height: 8),
        if (key != null && !widget.locked)
          ref.watch(eligibilityPreviewProvider(key)).when(
                skipLoadingOnRefresh: true,
                loading: () => const LinearProgressIndicator(),
                error: (_, __) => const SizedBox.shrink(),
                data: (p) => _PreviewLine(preview: p),
              ),
      ],
    );

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const SectionHeader('Eligibility'),
        if (widget.locked) ...[
          Text(
            'Eligibility cannot be changed after the event is published.',
            style: theme.textTheme.bodySmall,
          ),
          const SizedBox(height: 8),
          IgnorePointer(child: Opacity(opacity: 0.6, child: fields)),
        ] else
          fields,
      ],
    );
  }
}

class _PreviewLine extends StatelessWidget {
  const _PreviewLine({required this.preview});

  final EligibilityPreview preview;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    if (!preview.valid) {
      final problems = preview.fields.values.toList();
      return Text(
        problems.isEmpty ? 'Complete the eligibility choices.' : problems.join('\n'),
        style: TextStyle(color: scheme.error),
      );
    }
    final summary = preview.summary;
    if (summary == null) return const SizedBox.shrink();
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Icon(Icons.info_outline, size: 18, color: scheme.primary),
        const SizedBox(width: 6),
        Expanded(child: Text(summary)),
      ],
    );
  }
}
