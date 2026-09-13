import { Link } from 'expo-router';
import { useCallback, useEffect, useState } from 'react';
import {
  ActivityIndicator,
  Image,
  Pressable,
  RefreshControl,
  ScrollView,
  StyleSheet,
  Text,
  View,
  Platform
} from 'react-native';
import { Agent, Message } from '../src/types/api';
import { httpAPI } from '../src/services/api';
import { isMockMode, mockAPI } from '../src/services/mock-api';
import { leagues } from '../src/leagues';
import { colors } from '../src/theme';

const api = isMockMode ? mockAPI : httpAPI;

export default function Home() {
  const [agents, setAgents] = useState<Agent[]>([]);
  const [sportsMessages, setSportsMessages] = useState<Message[]>([]);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  const [all, setAll] = useState(false);
  const [selectedLeague, setSelectedLeague] = useState<typeof leagues[number] | null>(null);
  const [reporterID, setReporterID] = useState('');
  const [interval, setInterval] = useState(60);
  const [enabled, setEnabled] = useState(true);
  const [platforms, setPlatforms] = useState<string[]>(['facebook', 'x']);
  const [saving, setSaving] = useState(false);

  const load = useCallback(async () => {
    try {
      setError('');
      const [loadedAgents, allSports] = await Promise.all([
        api.listAgents(),
        api.listMessages('all_sports').catch(() => []),
      ]);
      setAgents(loadedAgents);
      setSportsMessages(allSports);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Unable to connect.');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const agentFor = (league: typeof leagues[number]) =>
    agents.find(agent => agent.assignment.toLowerCase().includes(league.name.toLowerCase().replace('uefa ', '')));

  const chooseReporter = (agent: Agent) => {
    setReporterID(agent.id);
    setInterval(agent.researchIntervalSeconds);
    setEnabled(agent.enabled);
    setPlatforms(agent.platforms);
  };

  const saveAssignment = async () => {
    const reporter = agents.find(agent => agent.id === reporterID);
    if (!selectedLeague || !reporter) return;
    try {
      setSaving(true);
      setError('');
      await api.updateAgent(reporter.id, { assignment: selectedLeague.name + ' news', enabled, platforms, researchIntervalSeconds: interval });
      setSelectedLeague(null);
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Could not save the reporter assignment.');
    } finally {
      setSaving(false);
    }
  };

  const feed = all ? leagues : leagues.filter(league => agentFor(league));
  const sportsDrafts = sportsMessages
    .filter((message): message is Message & { draft: NonNullable<Message['draft']> } => Boolean(message.draft))
    .sort((left, right) => new Date(right.createdAt).getTime() - new Date(left.createdAt).getTime())
    .slice(0, 8);

  return (
    <View style={s.container}>
      <ScrollView
        contentContainerStyle={s.page}
        showsVerticalScrollIndicator={false}
        refreshControl={
          <RefreshControl
            refreshing={loading}
            onRefresh={load}
            tintColor={colors.paper}
            colors={[colors.orange]}
          />
        }
      >
        {/* PREMIUM HEADER */}
        <View style={s.header}>
          <View style={s.headerTop}>
            <Text style={s.brand}>SPORTS DESK</Text>
            <Pressable style={s.searchButton}>
              <Text style={s.searchIcon}>⌕</Text>
            </Pressable>
          </View>

          <ScrollView
            horizontal
            showsHorizontalScrollIndicator={false}
            contentContainerStyle={s.logoRail}
          >
            {leagues.map(league => (
              <View key={league.id} style={s.logoItem}>
                <View style={s.logoRing}>
                  <Image source={{ uri: league.badgeURL }} style={s.logo} resizeMode="contain" />
                </View>
                <Text numberOfLines={1} style={s.logoLabel}>{league.name}</Text>
              </View>
            ))}
          </ScrollView>
        </View>

        {/* MAIN SHEET */}
        <View style={s.sheet}>
          {isMockMode && (
            <View style={s.demoContainer}>
              <Text style={s.demo}>DEMO MODE · Changes are not saved.</Text>
            </View>
          )}

          {!all && sportsDrafts.length > 0 && (
            <View style={s.carouselSection}>
              <View style={s.carouselHeading}><Text style={s.carouselLabel}>ALL SPORTS · LATEST</Text><Text style={s.carouselHint}>Swipe</Text></View>
              <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={s.carouselRail}>
                {sportsDrafts.map(message => {
                  const imageURL = message.draft.sources.find(source => source.imageUrl)?.imageUrl;
                  return <Link key={message.id} href={{ pathname: '/agents/[id]', params: { id: 'all_sports' } }} asChild><Pressable style={s.storyCard}>
                    {imageURL ? <Image source={{ uri: imageURL }} style={s.storyImage} /> : <View style={s.storyFallback}><Text style={s.storyFallbackText}>SPORTS</Text></View>}
                    <View style={s.storyOverlay}><Text numberOfLines={3} style={s.storyTitle}>{message.draft.headline}</Text><Text style={s.storyMeta}>{message.draft.claimStatus.toUpperCase()}</Text></View>
                  </Pressable></Link>;
                })}
              </ScrollView>
            </View>
          )}

          <View style={s.tabs}>
            <Pressable
              onPress={() => setAll(false)}
              style={[s.tab, !all && s.tabSelected]}
            >
              <Text style={[s.tabText, !all && s.tabTextSelected]}>Updates</Text>
            </Pressable>
            <Pressable
              onPress={() => setAll(true)}
              style={[s.tab, all && s.tabSelected]}
            >
              <Text style={[s.tabText, all && s.tabTextSelected]}>All Leagues</Text>
            </Pressable>
          </View>

          <View style={s.sectionHeader}>
            <Text style={s.title}>{all ? 'Competition feeds' : 'Your newsroom'}</Text>
          </View>

          {selectedLeague && (
            <View style={s.assign}>
              <View style={s.assignTop}>
                <Text style={s.assignTitle}>Assign {selectedLeague.name}</Text>
                <Pressable onPress={() => setSelectedLeague(null)}>
                  <Text style={s.assignClose}>×</Text>
                </Pressable>
              </View>
              <Text style={s.assignLabel}>REPORTER</Text>
              <View style={s.options}>
                {agents.map(agent => (
                  <Pressable key={agent.id} onPress={() => chooseReporter(agent)} style={[s.option, reporterID === agent.id && s.optionActive]}>
                    <Text style={[s.optionText, reporterID === agent.id && s.optionTextActive]}>{agent.id.replace('_', ' ')}</Text>
                  </Pressable>
                ))}
              </View>
              <Text style={s.assignLabel}>RESEARCH INTERVAL</Text>
              <View style={s.options}>
                {[30, 60, 300, 1800, 3600].map(value => (
                  <Pressable key={value} onPress={() => setInterval(value)} style={[s.option, interval === value && s.optionActive]}>
                    <Text style={[s.optionText, interval === value && s.optionTextActive]}>{value < 60 ? value + 's' : value < 3600 ? value / 60 + 'm' : '1h'}</Text>
                  </Pressable>
                ))}
              </View>
              <Text style={s.assignLabel}>DESTINATIONS & STATUS</Text>
              <View style={s.options}>
                {['facebook', 'x'].map(value => (
                  <Pressable key={value} onPress={() => setPlatforms(current => current.includes(value) ? current.length > 1 ? current.filter(item => item !== value) : current : [...current, value])} style={[s.option, platforms.includes(value) && s.optionActive]}>
                    <Text style={[s.optionText, platforms.includes(value) && s.optionTextActive]}>{value === 'x' ? 'X' : 'Facebook'}</Text>
                  </Pressable>
                ))}
                <Pressable onPress={() => setEnabled(value => !value)} style={[s.option, enabled && s.optionActive]}>
                  <Text style={[s.optionText, enabled && s.optionTextActive]}>{enabled ? 'Active' : 'Paused'}</Text>
                </Pressable>
              </View>
              <Pressable disabled={!reporterID || saving} onPress={saveAssignment} style={[s.assignSave, (!reporterID || saving) && s.assignSaveDisabled]}>
                <Text style={s.assignSaveText}>{saving ? 'SAVING…' : 'SAVE ASSIGNMENT'}</Text>
              </Pressable>
            </View>
          )}

          {loading ? (
            <ActivityIndicator color={colors.orange} style={s.loader} size="large" />
          ) : error ? (
            <View style={s.error}>
              <Text style={s.errorTitle}>Could not load your newsroom</Text>
              <Text style={s.errorMessage}>{error}</Text>
              <Pressable onPress={load} style={s.retryButton}>
                <Text style={s.retry}>TRY AGAIN</Text>
              </Pressable>
            </View>
          ) : (
            <View style={s.feedContainer}>
              {/* NO BROKER / EMPTY STATE CARD FOR UPDATES TAB */}
              {!all && feed.length === 0 ? (
                <View style={s.row}>
                  <View style={s.rowLogo}>
                    <Text style={s.emptyIcon}>✦</Text>
                  </View>
                  <View style={s.rowMain}>
                    <Text style={s.rowTitle}>No active agents</Text>
                    <Text style={s.rowCopy}>Switch to All Leagues to assign a reporter.</Text>
                  </View>
                </View>
              ) : (
                feed.map((league, index) => {
                  const agent = agentFor(league);
                  const copy = agent
                    ? agent.isRunning
                      ? 'Researching the latest developments'
                      : agent.pendingDraftCount
                        ? `${agent.pendingDraftCount} draft${agent.pendingDraftCount === 1 ? ' needs your review' : 's need your review'}`
                        : agent.enabled
                          ? 'Monitoring is active'
                          : 'Monitoring is paused'
                    : 'No reporter assigned yet';

                  const row = (
                    <Pressable style={({ pressed }) => [s.row, pressed && agent && s.pressed]}>
                      <View style={s.rowLogo}>
                        <Image source={{ uri: league.badgeURL }} style={s.rowLogoImage} resizeMode="contain" />
                        {agent?.isRunning && <View style={s.liveDot} />}
                      </View>

                      <View style={s.rowMain}>
                        <Text style={s.rowTitle}>{league.name}</Text>
                        <Text numberOfLines={1} style={[s.rowCopy, agent?.pendingDraftCount ? s.textHighlight : null]}>
                          {copy}
                        </Text>
                      </View>

                      <View style={s.rowEnd}>
                        {agent?.pendingDraftCount ? (
                          <View style={s.unread}>
                            <Text style={s.unreadText}>{agent.pendingDraftCount}</Text>
                          </View>
                        ) : (
                          <Text style={s.rowTime}>{index < 2 ? 'NOW' : league.country}</Text>
                        )}
                      </View>
                    </Pressable>
                  );

                  return agent ? (
                    <Link key={league.id} href={{ pathname: '/agents/[id]', params: { id: agent.id } }} asChild>
                      {row}
                    </Link>
                  ) : (
                    <Pressable key={league.id} onPress={() => { setSelectedLeague(league); if (agents[0]) chooseReporter(agents[0]); }}>
                      {row}
                    </Pressable>
                  );
                })
              )}
            </View>
          )}
        </View>
      </ScrollView>
    </View>
  );
}

const s = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: '#DF301C',
  },
  page: {
    flexGrow: 1,
    backgroundColor: '#F8FAFC',
  },
  header: {
    backgroundColor: '#DF301C',
    paddingTop: Platform.OS === 'ios' ? 60 : 40,
    paddingBottom: 48,
    borderBottomLeftRadius: 32,
    borderBottomRightRadius: 32,
    zIndex: 10,
  },
  headerTop: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    paddingHorizontal: 24,
  },
  brand: {
    color: '#FFFFFF',
    fontWeight: '900',
    fontSize: 24,
    letterSpacing: -0.5,
  },
  searchButton: {
    width: 44,
    height: 44,
    borderRadius: 22,
    backgroundColor: 'rgba(255,255,255,0.2)',
    alignItems: 'center',
    justifyContent: 'center',
  },
  searchIcon: {
    color: '#FFFFFF',
    fontSize: 22,
    lineHeight: 24,
  },
  logoRail: {
    paddingHorizontal: 24,
    paddingTop: 28,
    gap: 16,
  },
  logoItem: {
    width: 64,
    alignItems: 'center',
    gap: 8,
  },
  logoRing: {
    height: 60,
    width: 60,
    borderRadius: 30,
    backgroundColor: '#FFFFFF',
    alignItems: 'center',
    justifyContent: 'center',
    shadowColor: '#000',
    shadowOffset: { width: 0, height: 4 },
    shadowOpacity: 0.15,
    shadowRadius: 8,
    elevation: 5,
  },
  logo: {
    height: 36,
    width: 36,
  },
  logoLabel: {
    color: 'rgba(255,255,255,0.9)',
    fontSize: 10,
    fontWeight: '700',
    textAlign: 'center',
    textTransform: 'uppercase',
    letterSpacing: 0.5,
  },
  sheet: {
    backgroundColor: '#F8FAFC',
    marginTop: -24,
    borderTopLeftRadius: 32,
    borderTopRightRadius: 32,
    paddingTop: 24,
    paddingBottom: 40,
    minHeight: 600,
    zIndex: 20,
  },
  demoContainer: {
    paddingHorizontal: 24,
    marginBottom: 16,
  },
  demo: {
    backgroundColor: '#FEF2F2',
    color: '#EF4444',
    fontSize: 12,
    fontWeight: '800',
    paddingVertical: 10,
    paddingHorizontal: 16,
    borderRadius: 12,
    textAlign: 'center',
    overflow: 'hidden',
  },
  tabs: {
    flexDirection: 'row',
    backgroundColor: '#E2E8F0',
    borderRadius: 20,
    padding: 4,
    marginHorizontal: 24,
    marginBottom: 24,
  },
  tab: {
    flex: 1,
    alignItems: 'center',
    paddingVertical: 12,
    borderRadius: 16,
  },
  tabSelected: {
    backgroundColor: '#FFFFFF',
    shadowColor: '#000',
    shadowOffset: { width: 0, height: 2 },
    shadowOpacity: 0.05,
    shadowRadius: 4,
    elevation: 2,
  },
  tabText: {
    fontSize: 14,
    color: '#64748B',
    fontWeight: '700',
  },
  tabTextSelected: {
    color: '#0F172A',
    fontWeight: '900',
  },
  sectionHeader: {
    paddingHorizontal: 24,
    marginBottom: 12,
  },
  title: {
    fontSize: 22,
    fontWeight: '900',
    color: '#0F172A',
    letterSpacing: -0.3,
  },
  carouselSection: { marginBottom: 22 },
  carouselHeading: { paddingHorizontal: 24, marginBottom: 10, flexDirection: 'row', justifyContent: 'space-between' },
  carouselLabel: { color: '#DF301C', fontSize: 11, fontWeight: '900', letterSpacing: 1 },
  carouselHint: { color: '#94A3B8', fontSize: 11, fontWeight: '700' },
  carouselRail: { paddingHorizontal: 16, gap: 12 },
  storyCard: { width: 238, height: 150, borderRadius: 18, overflow: 'hidden', backgroundColor: '#0F172A', justifyContent: 'flex-end' },
  storyImage: { ...StyleSheet.absoluteFillObject, width: undefined, height: undefined },
  storyFallback: { ...StyleSheet.absoluteFillObject, backgroundColor: '#DF301C', alignItems: 'center', justifyContent: 'center' },
  storyFallbackText: { color: '#FFFFFF', fontSize: 18, fontWeight: '900', letterSpacing: 1 },
  storyOverlay: { padding: 13, backgroundColor: 'rgba(15,23,42,0.78)', gap: 5 },
  storyTitle: { color: '#FFFFFF', fontSize: 14, lineHeight: 18, fontWeight: '900' },
  storyMeta: { color: '#FF9100', fontSize: 9, letterSpacing: 1, fontWeight: '900' },
  feedContainer: {
    paddingHorizontal: 16,
  },
  row: {
    flexDirection: 'row',
    alignItems: 'center',
    backgroundColor: '#FFFFFF',
    padding: 16,
    marginVertical: 6,
    borderRadius: 20,
    gap: 16,
    shadowColor: '#000',
    shadowOffset: { width: 0, height: 2 },
    shadowOpacity: 0.03,
    shadowRadius: 8,
    elevation: 1,
  },
  pressed: {
    transform: [{ scale: 0.98 }],
    opacity: 0.8,
  },
  rowLogo: {
    height: 52,
    width: 52,
    borderRadius: 26,
    backgroundColor: '#F1F5F9',
    alignItems: 'center',
    justifyContent: 'center',
  },
  rowLogoImage: {
    height: 32,
    width: 32,
  },
  emptyIcon: {
    fontSize: 22,
    color: '#94A3B8',
  },
  liveDot: {
    position: 'absolute',
    right: -2,
    bottom: -2,
    width: 14,
    height: 14,
    borderRadius: 7,
    backgroundColor: '#10B981',
    borderWidth: 2,
    borderColor: '#FFFFFF',
  },
  rowMain: {
    flex: 1,
    justifyContent: 'center',
    gap: 4,
  },
  rowTitle: {
    fontSize: 16,
    fontWeight: '800',
    color: '#0F172A',
  },
  rowCopy: {
    fontSize: 13,
    color: '#64748B',
    fontWeight: '500',
  },
  textHighlight: {
    color: '#F97316',
    fontWeight: '600',
  },
  rowEnd: {
    minWidth: 48,
    alignItems: 'flex-end',
    justifyContent: 'center',
  },
  rowTime: {
    fontSize: 11,
    fontWeight: '800',
    color: '#94A3B8',
    textTransform: 'uppercase',
  },
  unread: {
    minWidth: 24,
    height: 24,
    borderRadius: 12,
    backgroundColor: '#EF4444',
    alignItems: 'center',
    justifyContent: 'center',
    paddingHorizontal: 8,
  },
  unreadText: {
    fontSize: 12,
    color: '#FFFFFF',
    fontWeight: '900',
  },
  assign: {
    marginHorizontal: 24,
    marginBottom: 14,
    padding: 16,
    gap: 9,
    borderRadius: 18,
    backgroundColor: '#0F172A',
  },
  assignTop: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between' },
  assignTitle: { color: '#FFFFFF', fontWeight: '900', fontSize: 16, flex: 1 },
  assignClose: { color: '#FF9100', fontSize: 28, lineHeight: 28 },
  assignLabel: { color: '#FF9100', fontSize: 9, fontWeight: '900', letterSpacing: .9, marginTop: 3 },
  options: { flexDirection: 'row', flexWrap: 'wrap', gap: 7 },
  option: { borderWidth: 1, borderColor: '#475569', borderRadius: 10, paddingHorizontal: 9, paddingVertical: 7 },
  optionActive: { backgroundColor: '#FF9100', borderColor: '#FF9100' },
  optionText: { color: '#FFFFFF', fontWeight: '800', fontSize: 10, textTransform: 'capitalize' },
  optionTextActive: { color: '#000000' },
  assignSave: { backgroundColor: '#DF301C', borderRadius: 11, padding: 12, alignItems: 'center', marginTop: 4 },
  assignSaveDisabled: { opacity: .5 },
  assignSaveText: { color: '#FFFFFF', fontWeight: '900', fontSize: 11, letterSpacing: .8 },
  loader: {
    marginTop: 60,
  },
  error: {
    marginHorizontal: 24,
    marginTop: 16,
    padding: 20,
    borderRadius: 20,
    backgroundColor: '#FEF2F2',
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
