import '../../../core/api/api_client.dart';
import '../../../core/api/json.dart';

const planAudiences = ['player', 'coach', 'organiser'];
const billingPeriods = ['monthly', 'quarterly', 'yearly'];

/// The closed list of benefit keys the backend accepts (docs/api.md, Premium).
/// Adding a key needs a backend change first; the server rejects unknown keys.
enum BenefitKind { flag, count }

class BenefitKey {
  const BenefitKey(this.key, this.kind, this.defaultLabel);

  final String key;
  final BenefitKind kind;
  final String defaultLabel;
}

const benefitKeys = <BenefitKey>[
  BenefitKey('consultation_listing', BenefitKind.flag, 'Offer paid consultations'),
  BenefitKey('profile_boost', BenefitKind.flag, 'Profile boost in search'),
  BenefitKey('advanced_stats', BenefitKind.flag, 'Advanced stats'),
  BenefitKey('see_profile_viewers', BenefitKind.flag, 'See who viewed your profile'),
  BenefitKey('advanced_search_filters', BenefitKind.flag, 'Advanced search filters'),
  BenefitKey('extra_communities', BenefitKind.count, 'Extra communities you can create'),
  BenefitKey('featured_events', BenefitKind.count, 'Featured events per month'),
  BenefitKey('verified_coach_badge', BenefitKind.flag, 'Verified coach badge'),
];

class PlanBenefit {
  const PlanBenefit({required this.key, required this.value, required this.label, this.sortOrder = 0});

  final String key;

  /// bool for flag benefits, int for count benefits.
  final Object? value;
  final String label;
  final int sortOrder;

  factory PlanBenefit.fromJson(Json j) =>
      PlanBenefit(key: asString(j['key']), value: j['value'], label: asString(j['label']), sortOrder: asInt(j['sortOrder']));

  Json toJson() => {'key': key, 'value': value, 'label': label, 'sortOrder': sortOrder};
}

class Plan {
  const Plan({
    this.id,
    required this.code,
    required this.audience,
    required this.name,
    required this.description,
    required this.pricePaise,
    required this.billingPeriod,
    this.storeProductIdAndroid,
    this.storeProductIdIos,
    this.benefits = const [],
    this.active = true,
    this.sortOrder = 0,
  });

  final int? id;
  final String code;
  final String audience;
  final String name;
  final String description;

  /// Display price; the store charges its own configured price.
  final int pricePaise;
  final String billingPeriod;
  final String? storeProductIdAndroid;
  final String? storeProductIdIos;
  final List<PlanBenefit> benefits;
  final bool active;
  final int sortOrder;

  factory Plan.fromJson(Json j) => Plan(
        id: asIntOrNull(j['id']),
        code: asString(j['code']),
        audience: asString(j['audience']),
        name: asString(j['name']),
        description: asString(j['description']),
        pricePaise: asInt(j['pricePaise']),
        billingPeriod: asString(j['billingPeriod']),
        storeProductIdAndroid: asStringOrNull(j['storeProductIdAndroid']),
        storeProductIdIos: asStringOrNull(j['storeProductIdIos']),
        benefits: asList(j['benefits'], PlanBenefit.fromJson),
        active: asBool(j['active'], true),
        sortOrder: asInt(j['sortOrder']),
      );

  /// Plan fields for create / update (benefits go through their own endpoint).
  Json toBody() => {
        'code': code,
        'audience': audience,
        'name': name,
        'description': description,
        'pricePaise': pricePaise,
        'billingPeriod': billingPeriod,
        'storeProductIdAndroid': storeProductIdAndroid,
        'storeProductIdIos': storeProductIdIos,
        'active': active,
        'sortOrder': sortOrder,
      };
}

class PlansApi {
  PlansApi(this._client);

  final ApiClient _client;

  /// `GET /admin/plans` (config.read).
  Future<List<Plan>> list() async => asList(await _client.get('/admin/plans'), Plan.fromJson);

  /// `POST /admin/plans` (plans.write).
  Future<Plan> create(Plan p) async => Plan.fromJson(asJson(await _client.post('/admin/plans', body: p.toBody())));

  /// `PUT /admin/plans/:id` (plans.write). `code` is not sent on update.
  Future<Plan> update(int id, Plan p) async =>
      Plan.fromJson(asJson(await _client.put('/admin/plans/$id', body: p.toBody()..remove('code'))));

  /// `PUT /admin/plans/:id/benefits` (plans.write). Replaces the whole list.
  Future<Plan> setBenefits(int id, List<PlanBenefit> benefits) async =>
      Plan.fromJson(asJson(await _client.put('/admin/plans/$id/benefits', body: [for (final b in benefits) b.toJson()])));
}
