import { Link } from 'expo-router';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { ActivityIndicator, Pressable, RefreshControl, ScrollView, StyleSheet, Text, View } from 'react-native';
import { httpAPI } from '../src/services/api';
import { isMockMode, mockAPI } from '../src/services/mock-api';
import { Agent } from '../src/types/api';

const api = isMockMode ? mockAPI : httpAPI;
type Filter = 'all' | 'review' | 'active';

function cadence(seconds: number) {
  if (seconds % 3600 === 0) return `Toutes les ${seconds / 3600} h`;
  return `Toutes les ${Math.max(1, Math.round(seconds / 60))} min`;
}

function relativeUpdate(value: string) {
  const minutes = Math.max(0, Math.round((Date.now() - new Date(value).getTime()) / 60000));
  if (minutes < 1) return 'à l’instant';
  if (minutes < 60) return `il y a ${minutes} min`;
  return `il y a ${Math.round(minutes / 60)} h`;
}

export default function Home() {
  const [agents, setAgents] = useState<Agent[]>([]);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [filter, setFilter] = useState<Filter>('all');

  const load = useCallback(async (refresh = false) => {
    refresh ? setRefreshing(true) : setLoading(true);
    setError('');
    try { setAgents(await api.listAgents()); }
    catch (cause) { setError(cause instanceof Error ? cause.message : 'Impossible de charger la rédaction.'); }
    finally { setLoading(false); setRefreshing(false); }
  }, []);

  useEffect(() => { void load(); }, [load]);

  const stats = useMemo(() => ({
    active: agents.filter((agent) => agent.isRunning).length,
    pending: agents.reduce((sum, agent) => sum + agent.pendingDraftCount, 0),
    ready: agents.filter((agent) => agent.enabled && !agent.isRunning).length,
  }), [agents]);
  const visibleAgents = useMemo(() => agents
    .filter((agent) => filter === 'all' || (filter === 'review' && agent.pendingDraftCount > 0) || (filter === 'active' && agent.isRunning))
    .sort((a, b) => Number(b.isRunning) - Number(a.isRunning) || b.pendingDraftCount - a.pendingDraftCount || a.assignment.localeCompare(b.assignment)), [agents, filter]);

  return <ScrollView contentContainerStyle={styles.container} refreshControl={<RefreshControl refreshing={refreshing} onRefresh={() => void load(true)} tintColor="#2563EB" />}>
    <View style={styles.hero}>
      <View style={styles.topline}><Text style={styles.eyebrow}>NEWSROOM · OPÉRATIONS</Text><View style={[styles.connection, isMockMode && styles.connectionDemo]}><View style={styles.connectionDot} /><Text style={styles.connectionText}>{isMockMode ? 'Démo' : 'Connecté'}</Text></View></View>
      <Text style={styles.title}>L’actualité, recherchée puis relue.</Text>
      <Text style={styles.subtitle}>Pilotez les recherches sportives, vérifiez les sources et relisez les brouillons avant toute diffusion.</Text>
    </View>

    {isMockMode ? <Text style={styles.demo}>Mode démo — les données sont fictives et aucune recherche n’est envoyée au serveur.</Text> : null}

    <View style={styles.metrics}>
      <Metric label="En recherche" value={stats.active} tone="blue" />
      <Metric label="À relire" value={stats.pending} tone="amber" />
      <Metric label="Disponibles" value={stats.ready} tone="slate" />
    </View>

    <View style={styles.sectionHeading}>
      <View><Text style={styles.sectionTitle}>Affectations</Text><Text style={styles.sectionHint}>{agents.length} agent{agents.length > 1 ? 's' : ''} configuré{agents.length > 1 ? 's' : ''}</Text></View>
      <Pressable accessibilityRole="button" onPress={() => void load(true)} hitSlop={10}><Text style={styles.refreshLabel}>Actualiser</Text></Pressable>
    </View>
    <View style={styles.filters}>
      <FilterButton label="Tout" selected={filter === 'all'} onPress={() => setFilter('all')} />
      <FilterButton label="À relire" selected={filter === 'review'} onPress={() => setFilter('review')} />
      <FilterButton label="Actifs" selected={filter === 'active'} onPress={() => setFilter('active')} />
    </View>

    {loading ? <ActivityIndicator size="large" color="#2563EB" style={styles.loader} /> : null}
    {!loading && error ? <View style={styles.errorPanel}><Text style={styles.errorTitle}>La rédaction est indisponible</Text><Text style={styles.errorText}>{error}</Text><Pressable style={styles.retryButton} onPress={() => void load()}><Text style={styles.retryText}>Réessayer</Text></Pressable></View> : null}
    {!loading && !error && visibleAgents.length === 0 ? <View style={styles.emptyPanel}><Text style={styles.emptyTitle}>Aucun agent dans cette vue</Text><Text style={styles.empty}>Changez le filtre ou ajoutez une affectation côté backend.</Text></View> : null}
    {!loading && !error && visibleAgents.map((agent) => <Link key={agent.id} href={{ pathname: '/agents/[id]', params: { id: agent.id } }} asChild><Pressable style={({ pressed }) => [styles.card, pressed && styles.pressed]} accessibilityRole="button">
      <View style={styles.cardTopline}><View style={[styles.statusDot, agent.isRunning ? styles.statusLive : agent.enabled ? styles.statusIdle : styles.statusPaused]} /><Text style={styles.statusText}>{agent.isRunning ? 'Recherche en cours' : agent.enabled ? 'Prêt à rechercher' : 'En pause'}</Text>{agent.pendingDraftCount > 0 ? <Text style={styles.draftCount}>{agent.pendingDraftCount} à relire</Text> : null}</View>
      <Text style={styles.cardTitle}>{agent.assignment}</Text>
      <Text style={styles.cardMeta}>{agent.language.toUpperCase()} · {cadence(agent.researchIntervalSeconds)} · mis à jour {relativeUpdate(agent.updatedAt)}</Text>
      <View style={styles.chips}>{agent.platforms.map((platform) => <Text key={platform} style={styles.chip}>{platform}</Text>)}</View>
    </Pressable></Link>)}
  </ScrollView>;
}

function Metric({ label, value, tone }: { label: string; value: number; tone: 'blue' | 'amber' | 'slate' }) {
  return <View style={[styles.metric, tone === 'blue' ? styles.metricBlue : tone === 'amber' ? styles.metricAmber : styles.metricSlate]}><Text style={styles.metricValue}>{value}</Text><Text style={styles.metricLabel}>{label}</Text></View>;
}

function FilterButton({ label, selected, onPress }: { label: string; selected: boolean; onPress: () => void }) {
  return <Pressable accessibilityRole="button" accessibilityState={{ selected }} onPress={onPress} style={[styles.filter, selected && styles.filterSelected]}><Text style={[styles.filterText, selected && styles.filterTextSelected]}>{label}</Text></Pressable>;
}

const styles = StyleSheet.create({
  container: { flexGrow: 1, padding: 20, paddingBottom: 42, backgroundColor: '#F5F7FB', gap: 14 },
  hero: { paddingTop: 18, paddingBottom: 7, gap: 9 }, topline: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', gap: 12 }, eyebrow: { color: '#2563EB', fontSize: 11, fontWeight: '800', letterSpacing: 1.4 }, connection: { flexDirection: 'row', alignItems: 'center', gap: 6, backgroundColor: '#EAF8F0', borderRadius: 20, paddingHorizontal: 9, paddingVertical: 5 }, connectionDemo: { backgroundColor: '#FFF3D6' }, connectionDot: { width: 6, height: 6, borderRadius: 4, backgroundColor: '#12A150' }, connectionText: { color: '#08783A', fontSize: 11, fontWeight: '800' }, title: { color: '#101828', fontSize: 31, lineHeight: 37, fontWeight: '800', maxWidth: 450 }, subtitle: { color: '#667085', fontSize: 15, lineHeight: 22, maxWidth: 510 }, demo: { color: '#745800', backgroundColor: '#FFF3D6', borderRadius: 12, padding: 13, lineHeight: 20 },
  metrics: { flexDirection: 'row', gap: 9 }, metric: { flex: 1, minHeight: 76, borderRadius: 13, padding: 12, justifyContent: 'space-between' }, metricBlue: { backgroundColor: '#EAF1FF' }, metricAmber: { backgroundColor: '#FFF4DB' }, metricSlate: { backgroundColor: '#EEF1F6' }, metricValue: { color: '#101828', fontSize: 25, lineHeight: 29, fontWeight: '800' }, metricLabel: { color: '#475467', fontSize: 11, fontWeight: '700' },
  sectionHeading: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center', marginTop: 11 }, sectionTitle: { color: '#101828', fontSize: 19, fontWeight: '800' }, sectionHint: { color: '#667085', fontSize: 12, marginTop: 2 }, refreshLabel: { color: '#2563EB', fontSize: 14, fontWeight: '800' }, filters: { flexDirection: 'row', gap: 8 }, filter: { borderWidth: 1, borderColor: '#D9DFEA', backgroundColor: 'white', borderRadius: 20, paddingHorizontal: 12, paddingVertical: 7 }, filterSelected: { borderColor: '#2563EB', backgroundColor: '#2563EB' }, filterText: { color: '#475467', fontSize: 13, fontWeight: '700' }, filterTextSelected: { color: 'white' },
  loader: { marginTop: 30 }, errorPanel: { gap: 8, padding: 16, borderRadius: 14, backgroundColor: '#FFF1F1' }, errorTitle: { color: '#8C1D18', fontWeight: '800', fontSize: 16 }, errorText: { color: '#A61B1B', lineHeight: 21 }, retryButton: { alignSelf: 'flex-start', backgroundColor: '#A61B1B', borderRadius: 8, paddingHorizontal: 12, paddingVertical: 9, marginTop: 3 }, retryText: { color: 'white', fontWeight: '700' }, emptyPanel: { backgroundColor: 'white', borderRadius: 14, borderWidth: 1, borderColor: '#E2E7F0', padding: 16, gap: 5 }, emptyTitle: { color: '#101828', fontWeight: '800' }, empty: { color: '#667085', lineHeight: 21 },
  card: { backgroundColor: 'white', borderRadius: 16, padding: 18, gap: 9, borderWidth: 1, borderColor: '#E2E7F0', shadowColor: '#344054', shadowOpacity: 0.05, shadowRadius: 10, elevation: 1 }, pressed: { opacity: 0.78 }, cardTopline: { flexDirection: 'row', alignItems: 'center', gap: 7 }, statusDot: { width: 8, height: 8, borderRadius: 8 }, statusLive: { backgroundColor: '#12A150' }, statusIdle: { backgroundColor: '#667085' }, statusPaused: { backgroundColor: '#D0D5DD' }, statusText: { color: '#475467', fontSize: 13, fontWeight: '700', flex: 1 }, draftCount: { color: '#704D00', backgroundColor: '#FFF3D6', borderRadius: 20, paddingHorizontal: 9, paddingVertical: 4, fontSize: 12, fontWeight: '800' }, cardTitle: { color: '#101828', fontSize: 20, lineHeight: 26, fontWeight: '800' }, cardMeta: { color: '#667085', fontSize: 13, lineHeight: 19 }, chips: { flexDirection: 'row', flexWrap: 'wrap', gap: 7, marginTop: 2 }, chip: { backgroundColor: '#EEF4FF', color: '#174EA6', borderRadius: 6, paddingHorizontal: 8, paddingVertical: 4, fontSize: 12, fontWeight: '700' },
});
