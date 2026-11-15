import 'package:flutter/material.dart';

import '../api/page.dart';
import 'api_error_view.dart';

/// A list over a cursor-paginated endpoint with "Load more". Call
/// `key.currentState?.reload()` after a filter change or a write.
class CursorList<T> extends StatefulWidget {
  const CursorList({
    super.key,
    required this.load,
    required this.itemBuilder,
    this.emptyText = 'Nothing to show.',
  });

  final Future<PageResult<T>> Function(String? cursor) load;
  final Widget Function(BuildContext context, T item) itemBuilder;
  final String emptyText;

  @override
  State<CursorList<T>> createState() => CursorListState<T>();
}

class CursorListState<T> extends State<CursorList<T>> {
  final List<T> _items = [];
  String? _cursor;
  bool _hasMore = true;
  bool _loading = false;
  Object? _error;
  int _generation = 0;

  List<T> get items => List.unmodifiable(_items);

  @override
  void initState() {
    super.initState();
    _loadMore();
  }

  Future<void> reload() async {
    _generation++;
    setState(() {
      _items.clear();
      _cursor = null;
      _hasMore = true;
      _error = null;
      _loading = false;
    });
    await _loadMore();
  }

  Future<void> _loadMore() async {
    if (_loading || !_hasMore) return;
    final gen = _generation;
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final page = await widget.load(_cursor);
      if (!mounted || gen != _generation) return;
      setState(() {
        _items.addAll(page.items);
        _cursor = page.nextCursor;
        _hasMore = page.hasMore;
        _loading = false;
      });
    } catch (e) {
      if (!mounted || gen != _generation) return;
      setState(() {
        _error = e;
        _loading = false;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_items.isEmpty) {
      if (_error != null) return ApiErrorView(error: _error!, onRetry: reload);
      if (_loading) return const Center(child: CircularProgressIndicator());
      return Center(child: Text(widget.emptyText));
    }
    return ListView.separated(
      itemCount: _items.length + 1,
      separatorBuilder: (_, __) => const Divider(height: 1),
      itemBuilder: (context, i) {
        if (i < _items.length) return widget.itemBuilder(context, _items[i]);
        if (_error != null) {
          return Padding(
            padding: const EdgeInsets.all(12),
            child: Column(children: [
              ApiErrorBanner(error: _error!),
              TextButton(onPressed: _loadMore, child: const Text('Retry')),
            ]),
          );
        }
        if (_loading) {
          return const Padding(padding: EdgeInsets.all(16), child: Center(child: CircularProgressIndicator()));
        }
        if (!_hasMore) {
          return const Padding(padding: EdgeInsets.all(16), child: Center(child: Text('End of list')));
        }
        return Center(child: TextButton(onPressed: _loadMore, child: const Text('Load more')));
      },
    );
  }
}
