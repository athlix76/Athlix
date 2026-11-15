import 'package:flutter/material.dart';

/// Edits a list of alias names as chips. Enter (or the add button) adds the
/// typed alias; the chip's delete icon removes it.
class AliasesField extends StatefulWidget {
  const AliasesField({super.key, required this.aliases, required this.onChanged});

  final List<String> aliases;
  final ValueChanged<List<String>> onChanged;

  @override
  State<AliasesField> createState() => _AliasesFieldState();
}

class _AliasesFieldState extends State<AliasesField> {
  final _text = TextEditingController();

  @override
  void dispose() {
    _text.dispose();
    super.dispose();
  }

  void _add() {
    final v = _text.text.trim();
    if (v.isEmpty) return;
    final exists = widget.aliases.any((a) => a.toLowerCase() == v.toLowerCase());
    _text.clear();
    if (!exists) widget.onChanged([...widget.aliases, v]);
  }

  @override
  Widget build(BuildContext context) {
    return Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      TextField(
        controller: _text,
        decoration: InputDecoration(
          labelText: 'Add alias (other spellings, old names)',
          border: const OutlineInputBorder(),
          suffixIcon: IconButton(icon: const Icon(Icons.add), onPressed: _add),
        ),
        onSubmitted: (_) => _add(),
      ),
      const SizedBox(height: 8),
      Wrap(spacing: 8, runSpacing: 8, children: [
        if (widget.aliases.isEmpty) const Text('No aliases.'),
        for (final a in widget.aliases)
          InputChip(
            label: Text(a),
            onDeleted: () => widget.onChanged(widget.aliases.where((x) => x != a).toList()),
          ),
      ]),
    ]);
  }
}
