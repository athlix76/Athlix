import '../../../core/api/api_client.dart';
import '../../../core/api/json.dart';
import '../../../core/models/listing.dart';

const eventTypes = ['tournament', 'trial', 'camp', 'match', 'other'];

/// `FeeRule` from docs/api.md. Money in paise, percentages in basis points.
class FeeRule {
  const FeeRule({
    this.id,
    required this.name,
    this.sportId,
    this.eventType,
    required this.percentBp,
    required this.minFeePaise,
    this.maxFeePaise,
    required this.freeEventFeePaise,
    required this.gstBp,
    required this.priority,
    required this.active,
    this.effectiveFrom,
    this.effectiveTo,
  });

  final int? id;
  final String name;
  final int? sportId;
  final String? eventType;
  final int percentBp;
  final int minFeePaise;

  /// null = no cap.
  final int? maxFeePaise;
  final int freeEventFeePaise;
  final int gstBp;
  final int priority;
  final bool active;
  final DateTime? effectiveFrom;
  final DateTime? effectiveTo;

  factory FeeRule.fromJson(Json j) => FeeRule(
        id: asIntOrNull(j['id']),
        name: asString(j['name']),
        sportId: asIntOrNull(j['sportId']),
        eventType: asStringOrNull(j['eventType']),
        percentBp: asInt(j['percentBp']),
        minFeePaise: asInt(j['minFeePaise']),
        maxFeePaise: asIntOrNull(j['maxFeePaise']),
        freeEventFeePaise: asInt(j['freeEventFeePaise']),
        gstBp: asInt(j['gstBp']),
        priority: asInt(j['priority']),
        active: asBool(j['active']),
        effectiveFrom: asDate(j['effectiveFrom']),
        effectiveTo: asDate(j['effectiveTo']),
      );

  /// Body for create / update (no id).
  Json toBody() => {
        'name': name,
        'sportId': sportId,
        'eventType': eventType,
        'percentBp': percentBp,
        'minFeePaise': minFeePaise,
        'maxFeePaise': maxFeePaise,
        'freeEventFeePaise': freeEventFeePaise,
        'gstBp': gstBp,
        'priority': priority,
        'active': active,
        'effectiveFrom': effectiveFrom?.toUtc().toIso8601String(),
        'effectiveTo': effectiveTo?.toUtc().toIso8601String(),
      };
}

class FeeRulesApi {
  FeeRulesApi(this._client);

  final ApiClient _client;

  /// `GET /admin/fee-rules` (config.read).
  Future<List<FeeRule>> list() async => asList(await _client.get('/admin/fee-rules'), FeeRule.fromJson);

  /// `POST /admin/fee-rules` (fees.write).
  Future<FeeRule> create(FeeRule r) async => FeeRule.fromJson(asJson(await _client.post('/admin/fee-rules', body: r.toBody())));

  /// `PUT /admin/fee-rules/:id` (fees.write). Only future quotes change.
  Future<FeeRule> update(int id, FeeRule r) async => FeeRule.fromJson(asJson(await _client.put('/admin/fee-rules/$id', body: r.toBody())));

  /// `POST /admin/fee-rules/preview` (config.read): the quote the server would
  /// give now for these inputs, using the saved rules.
  Future<ListingQuote> preview({int? sportId, String? eventType, required int entryFeePaise, required int maxSlots}) async =>
      ListingQuote.fromJson(asJson(await _client.post('/admin/fee-rules/preview', body: {
        'sportId': sportId,
        'eventType': eventType,
        'entryFeePaise': entryFeePaise,
        'maxSlots': maxSlots,
      })));
}
