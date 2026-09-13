import { useLocalSearchParams } from 'expo-router';
import { useCallback, useEffect, useState } from 'react';
import {
  ActivityIndicator,
  Pressable,
  RefreshControl,
  ScrollView,
  StyleSheet,
  Text,
  View,
  Platform
} from 'react-native';
import { httpAPI } from '../../src/services/api';
import { isMockMode, mockAPI } from '../../src/services/mock-api';
import { Message } from '../../src/types/api';
import { colors } from '../../src/theme';

const api = isMockMode ? mockAPI : httpAPI;

function messageTimestamp(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  return date.toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  });
}

export default function AgentScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const [messages, setMessages] = useState<Message[]>([]);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    if (!id) return;
    try {
      setError('');
      setMessages(await api.listMessages(id));
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Unable to load this conversation.');
    } finally {
      setLoading(false);
    }
  }, [id]);

  useEffect(() => {
    load();
  }, [load]);

  const newestFirstMessages = [...messages].sort(
    (left, right) => new Date(right.createdAt).getTime() - new Date(left.createdAt).getTime(),
  );

  return (
    <View style={s.container}>
      <ScrollView
        contentContainerStyle={s.page}
        showsVerticalScrollIndicator={false}
        refreshControl={
          <RefreshControl
            refreshing={loading}
            onRefresh={load}
            tintColor="#DF301C"
            colors={['#DF301C']}
          />
        }
      >
        {/* PREMIUM HERO CARD */}
        <View style={s.hero}>
          <View style={s.heroAvatar}>
            <Text style={s.heroAvatarText}>AI</Text>
          </View>
          <View style={s.heroContent}>
            <Text style={s.kicker}>NEWSROOM AI</Text>
            <Text style={s.heroTitle}>Your reporter{'\n'}is on the desk.</Text>
            <View style={s.heroBadge}>
              <Text style={s.heroSub}>{messages.length} updates in this thread</Text>
            </View>
          </View>
        </View>

        {isMockMode && (
          <View style={s.demo}>
            <Text style={s.demoText}>DEMO MODE · Changes are not saved.</Text>
          </View>
        )}

        {loading ? (
          <ActivityIndicator color="#DF301C" size="large" style={s.loader} />
        ) : error ? (
          <View style={s.error}>
            <Text style={s.errorTitle}>Could not load this reporter</Text>
            <Text style={s.errorMessage}>{error}</Text>
            <Pressable onPress={load} style={s.retryButton}>
              <Text style={s.retry}>TRY AGAIN</Text>
            </Pressable>
          </View>
        ) : messages.length === 0 ? (
          <View style={s.empty}>
            <View style={s.emptyIconContainer}>
              <Text style={s.emptyIcon}>✦</Text>
            </View>
            <Text style={s.emptyTitle}>No updates yet</Text>
            <Text style={s.emptyCopy}>New research findings and drafts will appear here once the agent begins monitoring.</Text>
          </View>
        ) : (
          <View style={s.threadContainer}>
            {newestFirstMessages.map(m => {
              const isAI = m.role === 'assistant';
              return (
                <View key={m.id} style={[s.thread, !isAI && s.threadSelf]}>
                  <View style={[s.avatar, !isAI && s.avatarSelf]}>
                    <Text style={[s.avatarText, !isAI && s.avatarTextSelf]}>
                      {isAI ? 'AI' : 'YOU'}
                    </Text>
                  </View>

                  <View style={[s.bubble, !isAI && s.bubbleSelf]}>
                    <View style={s.messageTop}>
                      <Text style={[s.role, !isAI && s.roleSelf]}>
                        {isAI ? 'NEWSROOM AI' : 'YOU'}
                      </Text>
                      <Text style={[s.time, !isAI && s.timeSelf]}>
                        {messageTimestamp(m.createdAt)}
                      </Text>
                    </View>

                    {m.draft ? (
                      <View style={s.draft}>
                        <View style={s.draftTop}>
                          <Text style={s.draftLabel}>DRAFT</Text>
                          <View style={[
                            s.statusBadge,
                            m.draft.reviewStatus === 'approved' ? s.badgeApproved :
                            m.draft.reviewStatus === 'rejected' ? s.badgeRejected :
                            s.badgePending
                          ]}>
                            <Text style={[
                              s.status,
                              m.draft.reviewStatus === 'approved' ? s.statusApproved :
                              m.draft.reviewStatus === 'rejected' ? s.statusRejected :
                              s.statusPending
                            ]}>
                              {m.draft.reviewStatus.toUpperCase()}
                            </Text>
                          </View>
                        </View>

                        <Text style={s.headline}>{m.draft.headline}</Text>
                        <Text style={s.copy}>{m.draft.facebookText}</Text>

                        <View style={s.evidence}>
                          <Text style={s.evidenceTitle}>EVIDENCE</Text>
                          <Text style={s.evidenceText}>
                            {m.draft.sources.length} source{m.draft.sources.length === 1 ? '' : 's'} · {m.draft.claimStatus}
                          </Text>
                        </View>
                      </View>
                    ) : (
                      <Text style={[s.statusMessage, !isAI && s.statusMessageSelf]}>
                        {m.text}
                      </Text>
                    )}
                  </View>
                </View>
              );
            })}
          </View>
        )}
      </ScrollView>
    </View>
  );
}

const s = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: '#F8FAFC',
  },
  page: {
    padding: 16,
    paddingTop: Platform.OS === 'ios' ? 60 : 40,
    paddingBottom: 60,
    backgroundColor: '#F8FAFC',
    flexGrow: 1,
  },
  hero: {
    backgroundColor: '#0F172A',
    borderRadius: 32,
    padding: 24,
    marginBottom: 24,
    flexDirection: 'row',
    alignItems: 'center',
    gap: 20,
    shadowColor: '#0F172A',
    shadowOffset: { width: 0, height: 8 },
    shadowOpacity: 0.15,
    shadowRadius: 16,
    elevation: 8,
  },
  heroAvatar: {
    width: 64,
    height: 64,
    borderRadius: 32,
    backgroundColor: '#DF301C',
    alignItems: 'center',
    justifyContent: 'center',
    borderWidth: 4,
    borderColor: 'rgba(255,255,255,0.1)',
  },
  heroAvatarText: {
    fontWeight: '900',
    color: '#FFFFFF',
    fontSize: 18,
  },
  heroContent: {
    flex: 1,
  },
  kicker: {
    color: '#F97316',
    fontSize: 11,
    fontWeight: '900',
    letterSpacing: 1.5,
    textTransform: 'uppercase',
  },
  heroTitle: {
    color: '#FFFFFF',
    fontSize: 26,
    lineHeight: 32,
    fontWeight: '900',
    letterSpacing: -0.5,
    marginTop: 4,
    marginBottom: 12,
  },
  heroBadge: {
    backgroundColor: 'rgba(255,255,255,0.1)',
    alignSelf: 'flex-start',
    paddingHorizontal: 12,
    paddingVertical: 6,
    borderRadius: 12,
  },
  heroSub: {
    color: '#E2E8F0',
    fontSize: 12,
    fontWeight: '700',
  },
  demo: {
    backgroundColor: '#FEF2F2',
    padding: 14,
    borderRadius: 16,
    marginBottom: 20,
    borderWidth: 1,
    borderColor: '#FECACA',
  },
  demoText: {
    fontWeight: '800',
    fontSize: 12,
    color: '#EF4444',
    textAlign: 'center',
  },
  loader: {
    marginTop: 40,
  },
  threadContainer: {
    gap: 24,
  },
  thread: {
    flexDirection: 'row',
    alignItems: 'flex-end',
    gap: 12,
    maxWidth: '92%',
  },
  threadSelf: {
    alignSelf: 'flex-end',
    flexDirection: 'row-reverse',
  },
  avatar: {
    width: 36,
    height: 36,
    borderRadius: 18,
    backgroundColor: '#1E293B',
    alignItems: 'center',
    justifyContent: 'center',
    shadowColor: '#000',
    shadowOffset: { width: 0, height: 2 },
    shadowOpacity: 0.1,
    shadowRadius: 4,
    elevation: 2,
  },
  avatarSelf: {
    backgroundColor: '#DF301C',
  },
  avatarText: {
    fontSize: 10,
    fontWeight: '900',
    color: '#FFFFFF',
  },
  avatarTextSelf: {
    color: '#FFFFFF',
  },
  bubble: {
    flex: 1,
    backgroundColor: '#FFFFFF',
    borderRadius: 24,
    borderBottomLeftRadius: 6,
    padding: 20,
    gap: 14,
    shadowColor: '#000',
    shadowOffset: { width: 0, height: 4 },
    shadowOpacity: 0.04,
    shadowRadius: 12,
    elevation: 3,
  },
  bubbleSelf: {
    backgroundColor: '#0F172A',
    borderBottomLeftRadius: 24,
    borderBottomRightRadius: 6,
  },
  messageTop: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
  },
  role: {
    fontSize: 10,
    fontWeight: '900',
    letterSpacing: 1,
    color: '#64748B',
  },
  roleSelf: {
    color: '#94A3B8',
  },
  time: {
    fontSize: 10,
    fontWeight: '700',
    color: '#94A3B8',
  },
  timeSelf: {
    color: '#64748B',
  },
  draft: {
    gap: 14,
  },
  draftTop: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    borderBottomWidth: 1,
    borderBottomColor: '#F1F5F9',
    paddingBottom: 12,
  },
  draftLabel: {
    fontSize: 11,
    fontWeight: '900',
    letterSpacing: 1.5,
    color: '#0F172A',
  },
  statusBadge: {
    paddingHorizontal: 10,
    paddingVertical: 4,
    borderRadius: 8,
  },
  badgePending: { backgroundColor: '#FEF3C7' },
  badgeApproved: { backgroundColor: '#D1FAE5' },
  badgeRejected: { backgroundColor: '#FEE2E2' },
  status: {
    fontSize: 9,
    fontWeight: '900',
    letterSpacing: 0.5,
  },
  statusPending: { color: '#92400E' },
  statusApproved: { color: '#065F46' },
  statusRejected: { color: '#991B1B' },
  headline: {
    fontSize: 22,
    lineHeight: 28,
    fontWeight: '900',
    letterSpacing: -0.5,
    color: '#0F172A',
  },
  copy: {
    fontSize: 15,
    lineHeight: 24,
    color: '#334155',
    fontWeight: '500',
  },
  evidence: {
    backgroundColor: '#F8FAFC',
    borderRadius: 12,
    padding: 14,
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginTop: 4,
  },
  evidenceTitle: {
    fontSize: 10,
    fontWeight: '900',
    letterSpacing: 1,
    color: '#64748B',
  },
  evidenceText: {
    fontSize: 11,
    fontWeight: '700',
    color: '#475569',
  },
  statusMessage: {
    fontSize: 15,
    lineHeight: 22,
    color: '#0F172A',
    fontWeight: '500',
  },
  statusMessageSelf: {
    color: '#FFFFFF',
  },
  empty: {
    backgroundColor: '#FFFFFF',
    borderRadius: 24,
    padding: 40,
    alignItems: 'center',
    gap: 12,
    shadowColor: '#000',
    shadowOffset: { width: 0, height: 4 },
    shadowOpacity: 0.03,
    shadowRadius: 12,
    elevation: 2,
    marginTop: 20,
  },
  emptyIconContainer: {
    width: 64,
    height: 64,
    borderRadius: 32,
    backgroundColor: '#FFF7ED',
    alignItems: 'center',
    justifyContent: 'center',
    marginBottom: 8,
  },
  emptyIcon: {
    fontSize: 32,
    color: '#F97316',
  },
  emptyTitle: {
    fontSize: 20,
    fontWeight: '900',
    color: '#0F172A',
  },
  emptyCopy: {
    fontSize: 14,
    color: '#64748B',
    textAlign: 'center',
    lineHeight: 22,
    fontWeight: '500',
  },
  error: {
    padding: 24,
    backgroundColor: '#FEF2F2',
    borderRadius: 20,
    borderWidth: 1,
    borderColor: '#FECACA',
    alignItems: 'center',
    gap: 8,
  },
  errorTitle: {
    fontWeight: '900',
    fontSize: 16,
    color: '#991B1B',
  },
  errorMessage: {
    color: '#B91C1C',
    textAlign: 'center',
    fontSize: 14,
  },
  retryButton: {
    marginTop: 12,
    backgroundColor: '#FEE2E2',
    paddingVertical: 10,
    paddingHorizontal: 20,
    borderRadius: 100,
  },
  retry: {
    color: '#991B1B',
    fontSize: 13,
    fontWeight: '900',
  }
});
