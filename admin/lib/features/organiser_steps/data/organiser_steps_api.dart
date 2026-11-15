import '../../../core/api/api_client.dart';
import '../../../core/api/json.dart';

const stepInputTypes = ['text', 'document', 'checkbox'];

/// `{id, stepCode, title, description, inputType, isRequired, sortOrder, active}`.
class OrganiserStep {
  const OrganiserStep({
    required this.id,
    required this.stepCode,
    required this.title,
    required this.description,
    required this.inputType,
    required this.isRequired,
    required this.sortOrder,
    required this.active,
  });

  final int id;
  final String stepCode;
  final String title;
  final String description;
  final String inputType;
  final bool isRequired;
  final int sortOrder;
  final bool active;

  factory OrganiserStep.fromJson(Json j) => OrganiserStep(
        id: asInt(j['id']),
        stepCode: asString(j['stepCode']),
        title: asString(j['title']),
        description: asString(j['description']),
        inputType: asString(j['inputType'], 'text'),
        isRequired: asBool(j['isRequired']),
        sortOrder: asInt(j['sortOrder']),
        active: asBool(j['active'], true),
      );
}

class OrganiserStepsApi {
  OrganiserStepsApi(this._client);

  final ApiClient _client;

  /// `GET /admin/organiser-steps` (config.read).
  Future<List<OrganiserStep>> list() async => asList(await _client.get('/admin/organiser-steps'), OrganiserStep.fromJson);

  /// `POST /admin/organiser-steps` (config.write).
  Future<OrganiserStep> create(Map<String, Object?> step) async =>
      OrganiserStep.fromJson(asJson(await _client.post('/admin/organiser-steps', body: step)));

  /// `PUT /admin/organiser-steps/:id` (config.write), partial.
  Future<OrganiserStep> update(int id, Map<String, Object?> changes) async =>
      OrganiserStep.fromJson(asJson(await _client.put('/admin/organiser-steps/$id', body: changes)));
}
