import 'dart:async';

import 'package:flutter/material.dart';

import '../../../core/format/format.dart';
import '../../../core/widgets/common.dart';
import '../application/proof_link.dart';
import '../data/location_review_api.dart';

/// Shows the proof behind a short-lived signed URL.
///
/// - Images render in place; PDFs open in a new browser tab (the CSP does not
///   allow framing other origins). Nothing is saved to disk or app storage.
/// - When the link's lifetime runs out the proof is hidden and the reviewer
///   presses "Show again", which calls [onRefetch] (re-GET of the detail, a new
///   signed URL and a new `location.proof_viewed` audit row).
/// - The decoded image is evicted from Flutter's in-memory image cache when the
///   link expires or the screen closes.
class ProofViewer extends StatefulWidget {
  const ProofViewer({super.key, required this.detail, required this.onRefetch});

  final LocationDetail detail;
  final VoidCallback onRefetch;

  @override
  State<ProofViewer> createState() => _ProofViewerState();
}

class _ProofViewerState extends State<ProofViewer> {
  Timer? _timer;
  bool _expired = false;
  DateTime? _expiresAt;

  @override
  void initState() {
    super.initState();
    _arm();
  }

  @override
  void didUpdateWidget(ProofViewer old) {
    super.didUpdateWidget(old);
    if (old.detail.proofUrl != widget.detail.proofUrl) {
      _evict(old.detail.proofUrl);
      _arm();
    }
  }

  @override
  void dispose() {
    _timer?.cancel();
    _evict(widget.detail.proofUrl);
    super.dispose();
  }

  void _arm() {
    _timer?.cancel();
    _expired = false;
    final url = widget.detail.proofUrl;
    _expiresAt = url == null ? null : proofLinkExpiry(url, widget.detail.fetchedAt);
    final at = _expiresAt;
    if (at == null) return;
    final left = at.difference(DateTime.now());
    if (left <= Duration.zero) {
      _expired = true;
      return;
    }
    _timer = Timer(left, _expire);
  }

  void _expire() {
    if (!mounted) return;
    _evict(widget.detail.proofUrl);
    setState(() => _expired = true);
  }

  void _evict(String? url) {
    if (url != null) NetworkImage(url).evict();
  }

  Widget _showAgain(String text) => Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Text(text),
        const SizedBox(height: 8),
        OutlinedButton.icon(
          onPressed: widget.onRefetch,
          icon: const Icon(Icons.visibility_outlined, size: 18),
          label: const Text('Show again (audited)'),
        ),
      ]);

  @override
  Widget build(BuildContext context) {
    final d = widget.detail;
    final small = Theme.of(context).textTheme.bodySmall;
    if (d.proofUrl == null) {
      return Text(d.proofPurgedAt != null
          ? 'Proof file deleted on ${formatDateTime(d.proofPurgedAt)} under the retention rule. Only the decision remains.'
          : 'No proof file (for example a DigiLocker native identity, or a file deleted after rejection).');
    }
    if (_expired) return _showAgain('The viewing link has expired.');
    final url = d.proofUrl!;
    final expiryNote = _expiresAt == null ? 'The link is short-lived.' : 'Link valid until ${TimeOfDay.fromDateTime(_expiresAt!).format(context)}.';
    if (d.isPdf) {
      return Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        OutlinedButton.icon(
          onPressed: () => openUrl(url),
          icon: const Icon(Icons.open_in_new, size: 18),
          label: const Text('Open PDF in a new tab'),
        ),
        const SizedBox(height: 4),
        Text('$expiryNote View it in the browser; do not download or forward it.', style: small),
      ]);
    }
    return Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
      ConstrainedBox(
        constraints: const BoxConstraints(maxHeight: 720),
        child: InteractiveViewer(
          maxScale: 5,
          child: Image.network(
            url,
            fit: BoxFit.contain,
            loadingBuilder: (context, child, progress) =>
                progress == null ? child : const Padding(padding: EdgeInsets.all(24), child: Center(child: CircularProgressIndicator())),
            errorBuilder: (context, _, __) => _showAgain('The proof could not be loaded (the link may have expired).'),
          ),
        ),
      ),
      const SizedBox(height: 4),
      Text('$expiryNote Scroll or pinch to zoom.', style: small),
    ]);
  }
}
