import 'package:flutter/material.dart';

import '../api/envelope.dart';
import 'async_value_view.dart';
import 'error_view.dart';

typedef PageFetcher<T> = Future<Paged<T>> Function(String? cursor);

/// Lets a screen refresh or patch a [CursorListView] it owns.
class CursorListController<T> {
  _CursorListViewState<T>? _state;

  List<T> get items => _state?._items ?? <T>[];

  Future<void> refresh() => _state?._reload() ?? Future<void>.value();

  void replaceWhere(bool Function(T item) test, T Function(T item) replace) =>
      _state?._replaceWhere(test, replace);

  void removeWhere(bool Function(T item) test) => _state?._removeWhere(test);

  void insertFirst(T item) => _state?._insertFirst(item);
}

/// Infinite list over a cursor-paginated endpoint (`?cursor=` + `meta.nextCursor`).
/// Change the widget [key] to restart from the first page (e.g. new filters).
class CursorListView<T> extends StatefulWidget {
  const CursorListView({
    super.key,
    required this.fetch,
    required this.itemBuilder,
    this.controller,
    this.emptyText = 'Nothing here yet.',
    this.header,
    this.onPage,
    this.padding,
    this.separated = false,
  });

  final PageFetcher<T> fetch;
  final Widget Function(BuildContext context, T item, int index) itemBuilder;
  final CursorListController<T>? controller;
  final String emptyText;
  final Widget? header;

  /// Called with every loaded page (e.g. to read `meta.unreadCount`).
  final void Function(Paged<T> page)? onPage;
  final EdgeInsetsGeometry? padding;
  final bool separated;

  @override
  State<CursorListView<T>> createState() => _CursorListViewState<T>();
}

class _CursorListViewState<T> extends State<CursorListView<T>> {
  final List<T> _items = <T>[];
  String? _cursor;
  bool _hasMore = true;
  bool _loading = false;
  bool _firstLoaded = false;
  Object? _error;
  int _generation = 0;
  bool _initialised = false;

  @override
  void initState() {
    super.initState();
    widget.controller?._state = this;
    _loadMore();
    _initialised = true;
  }

  @override
  void didUpdateWidget(covariant CursorListView<T> oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.controller != widget.controller) {
      oldWidget.controller?._state = null;
      widget.controller?._state = this;
    }
  }

  @override
  void dispose() {
    if (widget.controller?._state == this) widget.controller?._state = null;
    super.dispose();
  }

  Future<void> _reload() async {
    _generation++;
    setState(() {
      _items.clear();
      _cursor = null;
      _hasMore = true;
      _loading = false;
      _firstLoaded = false;
      _error = null;
    });
    await _loadMore();
  }

  Future<void> _loadMore() async {
    if (_loading || !_hasMore) return;
    final gen = _generation;
    _loading = true;
    _error = null;
    // Not from initState: the first build already shows a spinner.
    if (_initialised) setState(() {});
    try {
      final page = await widget.fetch(_cursor);
      if (!mounted || gen != _generation) return;
      widget.onPage?.call(page);
      setState(() {
        _items.addAll(page.items);
        _cursor = page.nextCursor;
        _hasMore = page.hasMore;
        _firstLoaded = true;
      });
    } catch (e) {
      if (!mounted || gen != _generation) return;
      setState(() => _error = e);
    } finally {
      if (mounted && gen == _generation) setState(() => _loading = false);
    }
  }

  void _replaceWhere(bool Function(T) test, T Function(T) replace) {
    setState(() {
      for (var i = 0; i < _items.length; i++) {
        if (test(_items[i])) _items[i] = replace(_items[i]);
      }
    });
  }

  void _removeWhere(bool Function(T) test) => setState(() => _items.removeWhere(test));

  void _insertFirst(T item) => setState(() => _items.insert(0, item));

  @override
  Widget build(BuildContext context) {
    if (!_firstLoaded) {
      if (_error != null) {
        return ErrorView(error: _error, onRetry: _loadMore);
      }
      return const Center(child: CircularProgressIndicator());
    }
    final header = widget.header;
    final headerCount = header == null ? 0 : 1;
    final footer = _hasMore || _error != null ? 1 : 0;
    if (_items.isEmpty && footer == 0) {
      return RefreshIndicator(
        onRefresh: _reload,
        child: ListView(
          physics: const AlwaysScrollableScrollPhysics(),
          padding: widget.padding,
          children: [
            if (header != null) header,
            SizedBox(height: 240, child: EmptyView(text: widget.emptyText)),
          ],
        ),
      );
    }
    return RefreshIndicator(
      onRefresh: _reload,
      child: ListView.builder(
        physics: const AlwaysScrollableScrollPhysics(),
        padding: widget.padding,
        itemCount: headerCount + _items.length + footer,
        itemBuilder: (context, i) {
          if (header != null && i == 0) return header;
          final index = i - headerCount;
          if (index >= _items.length) {
            if (_error != null) {
              return Padding(
                padding: const EdgeInsets.all(16),
                child: Column(
                  children: [
                    Text(errorMessage(_error), textAlign: TextAlign.center),
                    TextButton(onPressed: _loadMore, child: const Text('Try again')),
                  ],
                ),
              );
            }
            WidgetsBinding.instance.addPostFrameCallback((_) {
              if (mounted) _loadMore();
            });
            return const Padding(
              padding: EdgeInsets.all(16),
              child: Center(child: CircularProgressIndicator()),
            );
          }
          final child = widget.itemBuilder(context, _items[index], index);
          if (!widget.separated) return child;
          return Column(mainAxisSize: MainAxisSize.min, children: [child, const Divider(height: 1)]);
        },
      ),
    );
  }
}
