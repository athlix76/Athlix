import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/config/config_providers.dart';
import '../../../core/format/dates.dart';
import '../../../core/media/media_upload.dart';
import '../../../core/media/pick_image.dart';
import '../../../core/money/money.dart';
import '../../../core/router/routes.dart';
import '../../../core/widgets/async_value_view.dart';
import '../../../core/widgets/busy_button.dart';
import '../../../core/widgets/error_view.dart';
import '../../../core/widgets/pickers.dart';
import '../../../core/widgets/section.dart';
import '../../../core/widgets/sport_picker.dart';
import '../../auth/data/auth_api.dart';
import '../../listing_payment/presentation/quote_breakdown.dart';
import '../application/event_providers.dart';
import '../data/event_models.dart';
import 'event_eligibility_section.dart';

/// Create (no [eventId]) or edit an event. Published events lock fee inputs
/// and dates; only [EventEnums.publishedEditable] fields are sent.
class EventFormScreen extends ConsumerWidget {
  const EventFormScreen({super.key, this.eventId});

  final int? eventId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final id = eventId;
    if (id == null) return const _EventForm(existing: null);
    final value = ref.watch(eventProvider(id));
    return AsyncValueView<Event>(
      value: value,
      onRetry: () => ref.invalidate(eventProvider(id)),
      data: (e) => _EventForm(existing: e),
    );
  }
}

class _EventForm extends ConsumerStatefulWidget {
  const _EventForm({required this.existing});

  final Event? existing;

  @override
  ConsumerState<_EventForm> createState() => _EventFormState();
}

class _EventFormState extends ConsumerState<_EventForm> {
  final _form = GlobalKey<FormState>();
  late final EventInput _in =
      widget.existing == null ? EventInput() : EventInput.fromEvent(widget.existing!);
  late final _title = TextEditingController(text: _in.title);
  late final _description = TextEditingController(text: _in.description ?? '');
  late final _teamMin = TextEditingController(text: _in.teamSizeMin?.toString() ?? '');
  late final _teamMax = TextEditingController(text: _in.teamSizeMax?.toString() ?? '');
  late final _ageMin = TextEditingController(text: _in.ageMin?.toString() ?? '');
  late final _ageMax = TextEditingController(text: _in.ageMax?.toString() ?? '');
  late final _venue = TextEditingController(text: _in.venueName ?? '');
  late final _address = TextEditingController(text: _in.addressLine ?? '');
  late final _city = TextEditingController(text: _in.city ?? '');
  late final _district = TextEditingController(text: _in.district ?? '');
  late final _state = TextEditingController(text: _in.state ?? '');
  late final _pincode = TextEditingController(text: _in.pincode ?? '');
  late final _fee = TextEditingController(
    text: _in.entryFeePaise == null ? '' : paiseToRupeeInput(_in.entryFeePaise!),
  );
  late final _slots = TextEditingController(text: _in.maxSlots?.toString() ?? '');
  late final _payInstr = TextEditingController(text: _in.paymentInstructions ?? '');
  late final _rules = TextEditingController(text: _in.rules ?? '');
  late final _prize = TextEditingController(text: _in.prizeInfo ?? '');
  late final _contactName = TextEditingController(text: _in.contactName ?? '');
  final _contactPhone = TextEditingController();

  Timer? _debounce;
  QuoteKey? _quoteKey;
  bool _bannerChanged = false;

  bool get _isEdit => widget.existing != null;
  bool get _published => widget.existing?.status == 'published';

  /// Locked once published (fee inputs, dates and everything not in the allowlist).
  bool _locked(String field) => _published && !EventEnums.publishedEditable.contains(field);

  List<TextEditingController> get _all => [
        _title, _description, _teamMin, _teamMax, _ageMin, _ageMax, _venue, _address, _city, //
        _district, _state, _pincode, _fee, _slots, _payInstr, _rules, _prize, _contactName,
        _contactPhone,
      ];

  @override
  void initState() {
    super.initState();
    _fee.addListener(_scheduleQuote);
    _slots.addListener(_scheduleQuote);
    _scheduleQuote();
  }

  @override
  void dispose() {
    _debounce?.cancel();
    for (final c in _all) {
      c.dispose();
    }
    super.dispose();
  }

  /// Live listing-fee estimate, debounced so typing does not flood the API.
  void _scheduleQuote() {
    if (_published) return;
    _debounce?.cancel();
    _debounce = Timer(const Duration(milliseconds: 600), () {
      final sportId = _in.sportId;
      final fee = parseRupeesToPaise(_fee.text);
      final slots = int.tryParse(_slots.text.trim());
      final key = sportId == null || fee == null || slots == null || slots < 1
          ? null
          : (sportId: sportId, eventType: _in.eventType, entryFeePaise: fee, maxSlots: slots);
      if (mounted && key != _quoteKey) setState(() => _quoteKey = key);
    });
  }

  String? _t(TextEditingController c) => c.text.trim().isEmpty ? null : c.text.trim();
  int? _i(TextEditingController c) => int.tryParse(c.text.trim());

  String? _required(String? v) => (v == null || v.trim().isEmpty) ? 'Required' : null;

  Future<void> _pickBanner() async {
    final f = await pickImageFile();
    if (f == null) return;
    final uploader = ref.read(mediaUploaderProvider);
    final problem = uploader.validate(MediaPurpose.eventBanner, f);
    if (problem != null) {
      if (mounted) showInfoSnack(context, problem);
      return;
    }
    final id = await uploader.upload(MediaPurpose.eventBanner, f);
    setState(() {
      _in.bannerUploadId = id;
      _bannerChanged = true;
    });
  }

  Future<void> _save() async {
    if (!(_form.currentState?.validate() ?? false)) return;
    if (_in.sportId == null) {
      showInfoSnack(context, 'Choose a sport.');
      return;
    }
    if (_in.startsAt == null || _in.endsAt == null || _in.registrationDeadline == null) {
      showInfoSnack(context, 'Set the start, end and registration deadline.');
      return;
    }
    if (_in.paymentModes.isEmpty && (parseRupeesToPaise(_fee.text) ?? 0) > 0) {
      showInfoSnack(context, 'Choose how players pay you.');
      return;
    }
    final rule = _in.eligibility;
    if (!_published && rule.type != 'open') {
      final missing = rule.scope == 'state' ? rule.stateId == null : rule.districtId == null;
      if (missing) {
        showInfoSnack(context, 'Choose the ${rule.scope == 'state' ? 'state' : 'district'} players must be from.');
        return;
      }
    }
    final phone = _contactPhone.text.trim();
    _in
      ..title = _title.text.trim()
      ..description = _t(_description)
      ..teamSizeMin = _i(_teamMin)
      ..teamSizeMax = _i(_teamMax)
      ..ageMin = _i(_ageMin)
      ..ageMax = _i(_ageMax)
      ..venueName = _t(_venue)
      ..addressLine = _t(_address)
      ..city = _t(_city)
      ..district = _t(_district)
      ..state = _t(_state)
      ..pincode = _t(_pincode)
      ..entryFeePaise = parseRupeesToPaise(_fee.text)
      ..maxSlots = _i(_slots)
      ..paymentInstructions = _t(_payInstr)
      ..rules = _t(_rules)
      ..prizeInfo = _t(_prize)
      ..contactName = _t(_contactName)
      ..contactPhone = phone.isEmpty ? null : normaliseMobileInput(phone);
    if (!_bannerChanged) _in.bannerUploadId = null;

    final ctrl = ref.read(eventControllerProvider);
    final existing = widget.existing;
    if (existing == null) {
      final e = await ctrl.create(_in);
      if (mounted) context.pushReplacement(Routes.event(e.id));
    } else {
      await ctrl.update(existing.id, _in, status: existing.status);
      if (mounted) context.pop();
    }
  }

  Widget _dateButton(String label, DateTime? value, String field, ValueChanged<DateTime> set) {
    return ListTile(
      contentPadding: EdgeInsets.zero,
      enabled: !_locked(field),
      leading: const Icon(Icons.event),
      title: Text(label),
      subtitle: Text(value == null ? 'Not set' : formatDateTime(value)),
      onTap: () async {
        final d = await pickDateTime(context, initial: value);
        if (d != null) setState(() => set(d));
      },
    );
  }

  Widget _field(TextEditingController c, String label, String field,
      {String? Function(String?)? validator,
      TextInputType? keyboard,
      int maxLines = 1,
      String? prefix,
      String? helper,
      List<TextInputFormatter>? formatters}) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 12),
      child: TextFormField(
        controller: c,
        enabled: !_locked(field),
        keyboardType: keyboard,
        inputFormatters: formatters,
        maxLines: maxLines,
        minLines: 1,
        validator: validator,
        decoration: InputDecoration(labelText: label, prefixText: prefix, helperText: helper),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final config = ref.watch(appConfigProvider);
    final digits = [FilteringTextInputFormatter.digitsOnly];
    final maxSlots = config.eventMaxSlotsLimit;
    final allowFree = config.eventAllowFreeEvents ?? true;
    final minHours = config.eventMinHoursBeforeStart;
    final quoteKey = _quoteKey;
    return Scaffold(
      appBar: AppBar(title: Text(_isEdit ? 'Edit event' : 'Create event')),
      body: Form(
        key: _form,
        child: ListView(
          padding: const EdgeInsets.all(16),
          children: [
            if (_published)
              const Card(
                child: ListTile(
                  leading: Icon(Icons.lock_outline),
                  title: Text('This event is live'),
                  subtitle: Text('Dates, fee and slots are locked. You can still edit the description, '
                      'rules, prizes, venue, contact, payment instructions and banner.'),
                ),
              ),
            const SectionHeader('Basics'),
            IgnorePointer(
              ignoring: _locked('sportId'),
              child: SportDropdown(
                value: _in.sportId,
                onChanged: (v) {
                  setState(() => _in.sportId = v);
                  _scheduleQuote();
                },
              ),
            ),
            const SizedBox(height: 12),
            _field(_title, 'Title', 'title', validator: _required),
            _field(_description, 'Description', 'description', maxLines: 5),
            IgnorePointer(
              ignoring: _locked('eventType'),
              child: EnumDropdown(
                label: 'Event type',
                values: EventEnums.eventTypes,
                value: _in.eventType,
                onChanged: (v) {
                  setState(() => _in.eventType = v ?? _in.eventType);
                  _scheduleQuote();
                },
              ),
            ),
            const SizedBox(height: 12),
            IgnorePointer(
              ignoring: _locked('participantType'),
              child: EnumDropdown(
                label: 'Who registers',
                values: EventEnums.participantTypes,
                value: _in.participantType,
                labels: const {'individual': 'Individual players', 'team': 'Teams'},
                onChanged: (v) => setState(() => _in.participantType = v ?? _in.participantType),
              ),
            ),
            const SizedBox(height: 12),
            if (_in.participantType == 'team')
              Row(
                children: [
                  Expanded(child: _field(_teamMin, 'Min team size', 'teamSizeMin', keyboard: TextInputType.number, formatters: digits)),
                  const SizedBox(width: 8),
                  Expanded(child: _field(_teamMax, 'Max team size', 'teamSizeMax', keyboard: TextInputType.number, formatters: digits)),
                ],
              ),
            IgnorePointer(
              ignoring: _locked('competitionLevel'),
              child: EnumDropdown(
                label: 'Level',
                values: EventEnums.competitionLevels,
                value: _in.competitionLevel,
                onChanged: (v) => setState(() => _in.competitionLevel = v ?? _in.competitionLevel),
              ),
            ),
            const SizedBox(height: 12),
            IgnorePointer(
              ignoring: _locked('genderCategory'),
              child: EnumDropdown(
                label: 'Category',
                values: EventEnums.genderCategories,
                value: _in.genderCategory,
                onChanged: (v) => setState(() => _in.genderCategory = v ?? _in.genderCategory),
              ),
            ),
            const SizedBox(height: 12),
            Row(
              children: [
                Expanded(child: _field(_ageMin, 'Min age', 'ageMin', keyboard: TextInputType.number, formatters: digits)),
                const SizedBox(width: 8),
                Expanded(child: _field(_ageMax, 'Max age', 'ageMax', keyboard: TextInputType.number, formatters: digits)),
              ],
            ),
            const SectionHeader('When'),
            if (minHours != null && !_published)
              Text('Starts at least $minHours hours from now.', style: Theme.of(context).textTheme.bodySmall),
            _dateButton('Starts', _in.startsAt, 'startsAt', (d) => _in.startsAt = d),
            _dateButton('Ends', _in.endsAt, 'endsAt', (d) => _in.endsAt = d),
            _dateButton('Registration deadline', _in.registrationDeadline, 'registrationDeadline',
                (d) => _in.registrationDeadline = d),
            const SectionHeader('Where'),
            _field(_venue, 'Venue name', 'venueName', validator: _required),
            _field(_address, 'Address', 'addressLine'),
            _field(_city, 'City', 'city', validator: _required),
            _field(_district, 'District', 'district'),
            _field(_state, 'State', 'state', validator: _required),
            _field(_pincode, 'Pincode', 'pincode', keyboard: TextInputType.number, formatters: digits),
            EventEligibilitySection(input: _in, locked: _published, isNew: !_isEdit),
            const SectionHeader('Entry fee and slots'),
            const Text('Players pay the entry fee to you directly. Athlix never collects it.'),
            const SizedBox(height: 8),
            _field(
              _fee,
              'Entry fee per ${_in.participantType == 'team' ? 'team' : 'player'}',
              'entryFeePaise',
              prefix: '₹ ',
              keyboard: const TextInputType.numberWithOptions(decimal: true),
              validator: (v) {
                final p = parseRupeesToPaise(v ?? '');
                if (p == null) return 'Enter an amount (0 for free)';
                if (p == 0 && !allowFree) return 'Free events are not allowed';
                return null;
              },
            ),
            _field(
              _slots,
              'Max slots',
              'maxSlots',
              keyboard: TextInputType.number,
              formatters: digits,
              helper: maxSlots == null ? null : 'Up to $maxSlots',
              validator: (v) {
                final n = int.tryParse(v ?? '');
                if (n == null || n < 1) return 'At least 1';
                if (maxSlots != null && n > maxSlots) return 'At most $maxSlots';
                return null;
              },
            ),
            Wrap(
              spacing: 8,
              children: [
                for (final m in EventEnums.paymentModes)
                  FilterChip(
                    label: Text(humanize(m)),
                    selected: _in.paymentModes.contains(m),
                    onSelected: _locked('paymentModes')
                        ? null
                        : (on) => setState(() {
                            if (on) {
                              _in.paymentModes.add(m);
                            } else {
                              _in.paymentModes.remove(m);
                            }
                          }),
                  ),
              ],
            ),
            const SizedBox(height: 12),
            _field(_payInstr, 'Payment instructions', 'paymentInstructions', maxLines: 3),
            if (quoteKey != null)
              Card(
                child: Padding(
                  padding: const EdgeInsets.all(12),
                  child: ref.watch(listingEstimateProvider(quoteKey)).when(
                        skipLoadingOnReload: true,
                        data: (q) => Column(
                          crossAxisAlignment: CrossAxisAlignment.stretch,
                          children: [
                            Text('Listing fee estimate', style: Theme.of(context).textTheme.titleSmall),
                            const SizedBox(height: 8),
                            QuoteBreakdown(quote: q),
                          ],
                        ),
                        loading: () => const LinearProgressIndicator(),
                        error: (e, _) => Text(errorMessage(e)),
                      ),
                ),
              ),
            const SectionHeader('Details'),
            _field(_rules, 'Rules', 'rules', maxLines: 5),
            _field(_prize, 'Prizes', 'prizeInfo', maxLines: 3),
            _field(_contactName, 'Contact name', 'contactName', validator: _required),
            _field(
              _contactPhone,
              'Contact mobile',
              'contactPhone',
              keyboard: TextInputType.phone,
              prefix: '+91 ',
              helper: _isEdit
                  ? 'Leave empty to keep ${widget.existing?.contactPhoneMasked ?? 'the current number'}'
                  : 'Shown to players so they can pay and reach you',
              validator: (v) {
                final t = (v ?? '').trim();
                if (t.isEmpty) return _isEdit ? null : 'Required';
                return normaliseMobileInput(t) == null ? 'Enter a 10-digit mobile number' : null;
              },
            ),
            ListTile(
              contentPadding: EdgeInsets.zero,
              leading: const Icon(Icons.image_outlined),
              title: Text(_bannerChanged ? 'Banner uploaded' : 'Banner image'),
              trailing: BusyButton(label: 'Choose', style: BusyButtonStyle.text, onPressed: _pickBanner),
            ),
            const SizedBox(height: 16),
            BusyButton(label: _isEdit ? 'Save changes' : 'Save as draft', onPressed: _save),
            if (!_isEdit)
              const Padding(
                padding: EdgeInsets.only(top: 8),
                child: Text('After saving, pay the listing fee to submit the event.'),
              ),
          ],
        ),
      ),
    );
  }
}
