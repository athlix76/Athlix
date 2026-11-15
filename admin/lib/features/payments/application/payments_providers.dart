import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/providers.dart';
import '../data/listing_orders_api.dart';

final listingOrdersApiProvider = Provider<ListingOrdersApi>((ref) => ListingOrdersApi(ref.watch(apiClientProvider)));
