import { Link } from 'expo-router';
import { useCallback, useEffect, useState } from 'react';
import { ActivityIndicator, Pressable, RefreshControl, ScrollView, StyleSheet, Text, View } from 'react-native';
import { httpAPI } from '../src/services/api';
import { isMockMode, mockAPI } from '../src/services/mock-api';
import { Agent } from '../src/types/api';

const api = isMockMode ? mockAPI : httpAPI;

function cadence(seconds: number) {
  if (seconds % 3600 === 0) return `Toutes les ${seconds / 3600} h`;
  return `Toutes les ${Math.max(1, Math.round(seconds / 60))} min`;
}

export default function Home() {
  const [agents, setAgents] = useState<Agent[]>([]);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);

  const load = useCallback(async (refresh = false) => {
    refresh ? setRefreshing(true) : setLoading(true);
    setError('');
    try { setAgents(await api.listAgents()); }
    catch (cause) { setError(cause instanceof Error ? cause.message : 'Impossible de charger la rédaction.'); }
    finally { setLoading(false); setRefreshing(false); }
  }, []);

  useEffect(() => { void load(); }, [load]);

  return <ScrollView contentContainerStyle={styles.container} refreshControl={<RefreshControl refreshing={refreshing} onRefresh={() => void load(true)} tintColor="#276EF1" />}>
    <View style={styles.hero}>
      <Text style={styles.eyebrow}>SALLE DE RÉDACTION</Text>
      <Text style={styles.title}>Les agents à la recherche de l’actualité.</Text>
      <Text style={styles.subtitle}>Suivez l’état des recherches, les sources et les brouillons avant validation.</Text>
    </View>
    {isMockMode ? <Text style={styles.demo}>Mode démo — aucune action n’est enregistrée côté serveur.</Text> : null}
    <View style={styles.sectionHeading}><Text style={styles.sectionTitle}>Affectations</Text><Pressable accessibilityRole="button" onPress={() => void load(true)} hitSlop={10}><Text style={styles.refreshLabel}>Actualiser</Text></Pressable></View>
    {loading ? <ActivityIndicator size="large" color="#276EF1" style={styles.loader} /> : null}
    {!loading && error ? <View style={styles.errorPanel}><Text style={styles.errorText}>{error}</Text><Pressable style={styles.retryButton} onPress={() => void load()}><Text style={styles.retryText}>Réessayer</Text></Pressable></View> : null}
    {!loading && !error && agents.length === 0 ? <Text style={styles.empty}>Aucun agent n’est configuré pour le moment.</Text> : null}
    {!loading && !error && agents.map((agent) => <Link key={agent.id} href={{ pathname: '/agents/[id]', params: { id: agent.id } }} asChild><Pressable style={({ pressed }) => [styles.card, pressed && styles.pressed]} accessibilityRole="button">
      <View style={styles.cardTopline}><View style={[styles.statusDot, agent.isRunning ? styles.statusLive : styles.statusIdle]} /><Text style={styles.statusText}>{agent.isRunning ? 'Recherche en cours' : agent.enabled ? 'Prêt à rechercher' : 'En pause'}</Text>{agent.pendingDraftCount > 0 ? <Text style={styles.draftCount}>{agent.pendingDraftCount} à relire</Text> : null}</View>
      <Text style={styles.cardTitle}>{agent.assignment}</Text>
      <Text style={styles.cardMeta}>{agent.language.toUpperCase()} · {cadence(agent.researchIntervalSeconds)}</Text>
      <View style={styles.chips}>{agent.platforms.map((platform) => <Text key={platform} style={styles.chip}>{platform}</Text>)}</View>
    </Pressable></Link>)}
  </ScrollView>;
}

const styles = StyleSheet.create({
  container: { flexGrow: 1, padding: 20, paddingBottom: 42, backgroundColor: '#F7F8FC', gap: 14 }, hero: { paddingTop: 20, paddingBottom: 10, gap: 8 }, eyebrow: { color: '#276EF1', fontSize: 12, fontWeight: '800', letterSpacing: 1.4 }, title: { color: '#101828', fontSize: 30, lineHeight: 36, fontWeight: '800', maxWidth: 420 }, subtitle: { color: '#667085', fontSize: 15, lineHeight: 22, maxWidth: 510 }, demo: { color: '#745800', backgroundColor: '#FFF3CD', borderRadius: 10, padding: 12, lineHeight: 20 }, sectionHeading: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center', marginTop: 8 }, sectionTitle: { color: '#101828', fontSize: 18, fontWeight: '700' }, refreshLabel: { color: '#276EF1', fontSize: 14, fontWeight: '700' }, loader: { marginTop: 30 }, errorPanel: { gap: 12, padding: 16, borderRadius: 14, backgroundColor: '#FFF1F1' }, errorText: { color: '#A61B1B', lineHeight: 21 }, retryButton: { alignSelf: 'flex-start', backgroundColor: '#A61B1B', borderRadius: 8, paddingHorizontal: 12, paddingVertical: 9 }, retryText: { color: 'white', fontWeight: '700' }, empty: { color: '#667085', paddingVertical: 24 }, card: { backgroundColor: 'white', borderRadius: 16, padding: 18, gap: 9, borderWidth: 1, borderColor: '#E6E8EF', shadowColor: '#344054', shadowOpacity: 0.05, shadowRadius: 10, elevation: 1 }, pressed: { opacity: 0.78 }, cardTopline: { flexDirection: 'row', alignItems: 'center', gap: 7 }, statusDot: { width: 8, height: 8, borderRadius: 8 }, statusLive: { backgroundColor: '#14B87A' }, statusIdle: { backgroundColor: '#98A2B3' }, statusText: { color: '#475467', fontSize: 13, fontWeight: '600', flex: 1 }, draftCount: { color: '#553B00', backgroundColor: '#FFF3CD', borderRadius: 20, paddingHorizontal: 9, paddingVertical: 4, fontSize: 12, fontWeight: '700' }, cardTitle: { color: '#101828', fontSize: 20, lineHeight: 26, fontWeight: '700' }, cardMeta: { color: '#667085', fontSize: 13 }, chips: { flexDirection: 'row', flexWrap: 'wrap', gap: 7, marginTop: 2 }, chip: { backgroundColor: '#EEF4FF', color: '#174EA6', borderRadius: 6, paddingHorizontal: 8, paddingVertical: 4, fontSize: 12, fontWeight: '700' },
});
