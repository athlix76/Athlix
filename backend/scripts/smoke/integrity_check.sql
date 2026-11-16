-- Data integrity check (the schema has no foreign keys; integrity is kept by the services).
-- Every row must print 0. Run: docker compose exec -T mysql mysql -uathlix -pathlix-local athlix -N < backend/scripts/smoke/integrity_check.sql
SELECT 'event_registrations->events', COUNT(*) FROM event_registrations r LEFT JOIN events e ON e.id=r.event_id WHERE e.id IS NULL
UNION ALL SELECT 'event_registrations->users', COUNT(*) FROM event_registrations r LEFT JOIN users u ON u.id=r.user_id WHERE u.id IS NULL
UNION ALL SELECT 'events->organiser users', COUNT(*) FROM events e LEFT JOIN users u ON u.id=e.organiser_id WHERE u.id IS NULL
UNION ALL SELECT 'posts->author users', COUNT(*) FROM posts p LEFT JOIN users u ON u.id=p.author_id WHERE u.id IS NULL
UNION ALL SELECT 'post_comments->posts', COUNT(*) FROM post_comments c LEFT JOIN posts p ON p.id=c.post_id WHERE p.id IS NULL
UNION ALL SELECT 'post_likes->posts', COUNT(*) FROM post_likes l LEFT JOIN posts p ON p.id=l.post_id WHERE p.id IS NULL
UNION ALL SELECT 'messages->conversations', COUNT(*) FROM messages m LEFT JOIN conversations c ON c.id=m.conversation_id WHERE c.id IS NULL
UNION ALL SELECT 'conversation_members->conversations', COUNT(*) FROM conversation_members m LEFT JOIN conversations c ON c.id=m.conversation_id WHERE c.id IS NULL
UNION ALL SELECT 'conversation_members->users', COUNT(*) FROM conversation_members m LEFT JOIN users u ON u.id=m.user_id WHERE u.id IS NULL
UNION ALL SELECT 'follows->users', COUNT(*) FROM follows f LEFT JOIN users a ON a.id=f.follower_id LEFT JOIN users b ON b.id=f.followee_id WHERE a.id IS NULL OR b.id IS NULL
UNION ALL SELECT 'follows involving deleted users', COUNT(*) FROM follows f JOIN users a ON a.id=f.follower_id JOIN users b ON b.id=f.followee_id WHERE a.status='deleted' OR b.status='deleted'
UNION ALL SELECT 'listing_orders->events', COUNT(*) FROM listing_orders o LEFT JOIN events e ON e.id=o.event_id WHERE e.id IS NULL
UNION ALL SELECT 'profiles->users', COUNT(*) FROM profiles p LEFT JOIN users u ON u.id=p.user_id WHERE u.id IS NULL
UNION ALL SELECT 'notifications->users', COUNT(*) FROM notifications n LEFT JOIN users u ON u.id=n.user_id WHERE u.id IS NULL
UNION ALL SELECT 'device_tokens of deleted users', COUNT(*) FROM device_tokens d JOIN users u ON u.id=d.user_id WHERE u.status='deleted'
UNION ALL SELECT 'visible messages of deleted users', COUNT(*) FROM messages m JOIN users u ON u.id=m.sender_id WHERE u.status='deleted' AND m.status='visible'
UNION ALL SELECT 'registrations_count drift', COUNT(*) FROM events e WHERE e.registrations_count <> (SELECT COUNT(*) FROM event_registrations r WHERE r.event_id=e.id AND r.status='registered')
UNION ALL SELECT 'followers_count drift', COUNT(*) FROM profiles p WHERE p.followers_count <> (SELECT COUNT(*) FROM follows f WHERE f.followee_id=p.user_id);
