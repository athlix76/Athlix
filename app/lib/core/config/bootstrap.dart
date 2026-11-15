import '../json/json_utils.dart';
import '../models/policy.dart';
import '../models/sport.dart';
import 'app_config.dart';

/// `GET /bootstrap`: public config, active policies, sports, minimum versions.
class Bootstrap {
  const Bootstrap({
    required this.config,
    required this.policies,
    required this.sports,
    required this.minVersionAndroid,
    required this.minVersionIos,
    required this.maintenance,
    required this.serverTime,
  });

  final AppConfig config;
  final List<PolicyRef> policies;
  final List<Sport> sports;
  final String? minVersionAndroid;
  final String? minVersionIos;
  final bool maintenance;
  final DateTime? serverTime;

  factory Bootstrap.fromJson(Map<String, dynamic> json) {
    final config = AppConfig(asMap(json['config']));
    final minVersion = asMap(json['minVersion']);
    return Bootstrap(
      config: config,
      policies: asList(json['policies'], PolicyRef.fromJson),
      sports: asList(json['sports'], Sport.fromJson),
      // Fall back to the public config keys when minVersion is absent.
      minVersionAndroid:
          asStringOrNull(minVersion['android']) ?? config.string('app.min_version_android'),
      minVersionIos: asStringOrNull(minVersion['ios']) ?? config.string('app.min_version_ios'),
      maintenance: asBool(json['maintenance']) || (config.boolean('app.maintenance_mode') ?? false),
      serverTime: asDateTimeOrNull(json['serverTime']),
    );
  }

  Bootstrap copyWith({bool? maintenance}) => Bootstrap(
        config: config,
        policies: policies,
        sports: sports,
        minVersionAndroid: minVersionAndroid,
        minVersionIos: minVersionIos,
        maintenance: maintenance ?? this.maintenance,
        serverTime: serverTime,
      );

  Sport? sportById(int? id) {
    if (id == null) return null;
    for (final s in sports) {
      if (s.id == id) return s;
    }
    return null;
  }

  PolicyRef? policy(String docType) {
    for (final p in policies) {
      if (p.docType == docType) return p;
    }
    return null;
  }
}
