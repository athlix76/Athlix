import 'package:flutter_test/flutter_test.dart';

import 'package:athlix/features/organiser/data/organiser_models.dart';
import 'package:athlix/features/organiser/presentation/organiser_verification_screen.dart';

VerificationStep step(int id, String type, {bool required = false}) => VerificationStep.fromJson({
      'stepId': id,
      'stepCode': 's$id',
      'title': 'Step $id',
      'description': '',
      'inputType': type,
      'isRequired': required,
      'submission': null,
    });

void main() {
  final steps = [
    step(1, 'text', required: true),
    step(2, 'document'), // optional, nothing uploaded
    step(3, 'document'), // optional, uploaded
    step(4, 'checkbox', required: true),
  ];

  test('an optional document step without a document is left out of PUT /organiser/verification', () {
    final out = buildStepInputs(steps, text: {1: '  Chennai FC  '}, checked: {4: true}, docIds: {3: 77});
    final json = out.map((s) => s.toJson()).toList();

    expect(json.map((j) => j['stepId']), [1, 3, 4]); // step 2 omitted: the server would reject it (422)
    expect(json[0]['valueText'], 'Chennai FC');
    expect(json[1]['documentUploadId'], 77);
    expect(json[2]['checked'], true);
    expect(json.every((j) => j.containsKey('documentUploadId') ? j['documentUploadId'] != null : true), isTrue);
  });

  test('text and checkbox steps are always sent, even when empty', () {
    final out = buildStepInputs(steps, text: const {}, checked: const {}, docIds: const {});
    expect(out.map((s) => s.stepId), [1, 4]);
    expect(out.last.toJson()['checked'], false);
  });
}
