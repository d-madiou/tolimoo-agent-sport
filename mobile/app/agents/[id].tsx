import { useLocalSearchParams } from 'expo-router';
import { useCallback, useEffect, useState } from 'react';
import { ActivityIndicator, Linking, Pressable, RefreshControl, ScrollView, StyleSheet, Text, View } from 'react-native';
import { APIError, httpAPI } from '../../src/services/api';
import { isMockMode, mockAPI } from '../../src/services/mock-api';
import { Agent, ClaimStatus, DraftStatus, Message, Run } from '../../src/types/api';

const api = isMockMode ? mockAPI : httpAPI;
const claimLabel: Record<ClaimStatus, string> = { official: 'Source officielle', reported: 'Rapporté par la presse', unverified: 'À confirmer' };
const reviewLabel: Record<DraftStatus, string> = { pending: 'En attente de relecture', approved: 'Approuvé', rejected: 'Écarté' };
const roleLabel: Record<Message['role'], string> = { user: 'Éditeur', assistant: 'Assistant', system: 'Système' };
const formatDate = (value: string) => new Intl.DateTimeFormat('fr-FR', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value));
const safeOpen = (url: string) => { if (/^https?:\/\//i.test(url)) void Linking.openURL(url); };

function researchError(cause: unknown) {
  if (cause instanceof APIError) {
    if (cause.code === 'configuration_error') return 'La recherche n’est pas prête : configurez les accès Exa et OpenRouter côté backend.';
    if (cause.code === 'queue_full') return 'La file de recherche est pleine. Réessayez dans quelques instants.';
    if (cause.code === 'conflict') return 'Une recherche est déjà active pour cet agent.';
  }
  return cause instanceof Error ? cause.message : 'Impossible de démarrer la recherche.';
}

export default function AgentScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const [agent, setAgent] = useState<Agent | null>(null);
  const [messages, setMessages] = useState<Message[]>([]);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [starting, setStarting] = useState(false);
  const [run, setRun] = useState<Run | null>(null);

  const load = useCallback(async (refresh = false) => {
    if (!id) return;
    refresh ? setRefreshing(true) : setLoading(true);
    setError('');
    try {
      const [agents, nextMessages] = await Promise.all([api.listAgents(), api.listMessages(id)]);
      setAgent(agents.find((item) => item.id === id) ?? null);
      setMessages(nextMessages);
    } catch (cause) { setError(cause instanceof Error ? cause.message : 'Impossible de charger cette affectation.'); }
    finally { setLoading(false); setRefreshing(false); }
  }, [id]);

  useEffect(() => { void load(); }, [load]);
  useEffect(() => {
    if (!agent?.isRunning) return;
    const timer = setInterval(() => void load(true), 5000);
    return () => clearInterval(timer);
  }, [agent?.isRunning, load]);

  const startResearch = async () => {
    if (!id || starting || agent?.isRunning) return;
    setStarting(true); setError('');
    try { setRun(await api.startRun(id)); await load(true); }
    catch (cause) {
      if (cause instanceof APIError && cause.code === 'conflict') await load(true);
      setError(researchError(cause));
    } finally { setStarting(false); }
  };

  const researchActive = Boolean(agent?.isRunning || starting);
  return <ScrollView contentContainerStyle={styles.container} refreshControl={<RefreshControl refreshing={refreshing} onRefresh={() => void load(true)} tintColor="#2563EB" />}>
    {loading ? <ActivityIndicator size="large" color="#2563EB" style={styles.loader} /> : null}
    {!loading && error ? <View style={styles.errorPanel}><Text style={styles.errorText}>{error}</Text><Pressable onPress={() => void load()}><Text style={styles.retryText}>Réessayer</Text></Pressable></View> : null}
    {!loading && agent ? <>
      <View style={styles.hero}><Text style={styles.eyebrow}>AGENT · {agent.language.toUpperCase()}</Text><Text style={styles.title}>{agent.assignment}</Text><Text style={styles.subtitle}>Les brouillons restent internes : aucune publication externe n’est déclenchée par cette interface.</Text><View style={styles.statusLine}><View style={[styles.statusDot, researchActive ? styles.statusLive : styles.statusIdle]} /><Text style={styles.statusText}>{researchActive ? 'Recherche en cours — actualisation automatique' : 'Prêt pour une recherche manuelle'}</Text></View></View>
      {isMockMode ? <Text style={styles.demo}>Mode démo — la recherche est simulée.</Text> : null}
      {run ? <Text style={styles.runNotice}>{agent.isRunning ? `Recherche lancée à ${formatDate(run.startedAt)}.` : `Demande envoyée à ${formatDate(run.startedAt)}. Consultez le journal pour son résultat.`}</Text> : null}
      <Pressable accessibilityRole="button" disabled={researchActive || !agent.enabled} onPress={() => void startResearch()} style={[styles.startButton, (researchActive || !agent.enabled) && styles.startDisabled]}>{starting ? <ActivityIndicator color="white" /> : <Text style={styles.startText}>{researchActive ? 'Recherche en cours' : 'Lancer une recherche'}</Text>}</Pressable>
      {!agent.enabled ? <Text style={styles.disabledText}>Cet agent est actuellement en pause.</Text> : null}
      <View style={styles.disclosure}><Text style={styles.disclosureTitle}>Comment fonctionne cette recherche</Text><Text style={styles.disclosureText}>Elle examine une fenêtre récente, conserve les liens d’origine et crée un brouillon uniquement si des sources attribuables le justifient.</Text></View>
      <View style={styles.sectionHeading}><Text style={styles.sectionTitle}>Journal de recherche</Text><Text style={styles.count}>{messages.length} élément{messages.length > 1 ? 's' : ''}</Text></View>
      {messages.length === 0 ? <Text style={styles.empty}>Aucun message pour l’instant. Lancez une recherche pour créer un journal attribué aux sources.</Text> : null}
      {messages.map((message) => <View key={message.id} style={styles.message}><View style={styles.messageMeta}><Text style={styles.role}>{roleLabel[message.role]}</Text><Text style={styles.date}>{formatDate(message.createdAt)}</Text></View><Text style={styles.messageText}>{message.text}</Text>{message.draft ? <View style={styles.draft}><Text style={styles.draftEyebrow}>{reviewLabel[message.draft.reviewStatus]} · {claimLabel[message.draft.claimStatus]}</Text><Text style={styles.headline}>{message.draft.headline}</Text><Text style={styles.platform}>FACEBOOK</Text><Text style={styles.copy}>{message.draft.facebookText}</Text><Text style={styles.platform}>X</Text><Text style={styles.copy}>{message.draft.xText}</Text><Text style={styles.sourceHeading}>Sources ({message.draft.sources.length})</Text>{message.draft.sources.map((source) => <Pressable key={source.id} onPress={() => safeOpen(source.url)} style={styles.source} accessibilityRole="link"><Text style={styles.sourceTitle}>{source.title}</Text><Text style={styles.sourceUrl} numberOfLines={1}>{source.url}</Text><Text style={styles.sourceMeta}>{source.publishedAt ? `Publié ${formatDate(source.publishedAt)}` : `Consulté ${formatDate(source.retrievedAt)}`}</Text></Pressable>)}</View> : null}</View>)}
    </> : null}
  </ScrollView>;
}

const styles = StyleSheet.create({
  container: { flexGrow: 1, padding: 20, paddingBottom: 42, backgroundColor: '#F5F7FB', gap: 14 }, loader: { marginTop: 38 }, hero: { paddingTop: 8, gap: 8 }, eyebrow: { color: '#2563EB', fontSize: 12, fontWeight: '800', letterSpacing: 1.3 }, title: { color: '#101828', fontSize: 28, lineHeight: 35, fontWeight: '800' }, subtitle: { color: '#667085', fontSize: 15, lineHeight: 22 }, statusLine: { flexDirection: 'row', alignItems: 'center', gap: 7, marginTop: 5 }, statusDot: { width: 8, height: 8, borderRadius: 8 }, statusLive: { backgroundColor: '#12A150' }, statusIdle: { backgroundColor: '#667085' }, statusText: { color: '#475467', fontSize: 13, fontWeight: '600' }, demo: { color: '#745800', backgroundColor: '#FFF3CD', borderRadius: 10, padding: 12 }, runNotice: { color: '#174EA6', backgroundColor: '#EEF4FF', borderRadius: 10, padding: 12, lineHeight: 20 }, startButton: { minHeight: 50, alignItems: 'center', justifyContent: 'center', backgroundColor: '#2563EB', borderRadius: 12, paddingHorizontal: 18 }, startDisabled: { backgroundColor: '#9BBEF8' }, startText: { color: 'white', fontSize: 16, fontWeight: '800' }, disabledText: { color: '#667085', fontSize: 13 }, disclosure: { backgroundColor: '#EEF3FA', borderRadius: 12, padding: 13, gap: 4 }, disclosureTitle: { color: '#344054', fontSize: 13, fontWeight: '800' }, disclosureText: { color: '#667085', fontSize: 13, lineHeight: 19 }, sectionHeading: { flexDirection: 'row', alignItems: 'baseline', justifyContent: 'space-between', marginTop: 12 }, sectionTitle: { color: '#101828', fontSize: 18, fontWeight: '700' }, count: { color: '#667085', fontSize: 13 }, empty: { color: '#667085', backgroundColor: 'white', borderRadius: 14, borderWidth: 1, borderColor: '#E6E8EF', padding: 16, lineHeight: 22 }, errorPanel: { gap: 10, padding: 16, borderRadius: 14, backgroundColor: '#FFF1F1' }, errorText: { color: '#A61B1B', lineHeight: 21 }, retryText: { color: '#A61B1B', fontWeight: '800' }, message: { backgroundColor: 'white', borderRadius: 16, padding: 16, gap: 10, borderWidth: 1, borderColor: '#E2E7F0' }, messageMeta: { flexDirection: 'row', justifyContent: 'space-between', gap: 10 }, role: { color: '#2563EB', fontSize: 12, fontWeight: '800', letterSpacing: 0.7 }, date: { color: '#98A2B3', fontSize: 12 }, messageText: { color: '#344054', lineHeight: 21 }, draft: { gap: 9, backgroundColor: '#F8FAFF', borderLeftColor: '#2563EB', borderLeftWidth: 3, borderRadius: 6, padding: 13 }, draftEyebrow: { color: '#475467', fontSize: 12, fontWeight: '700' }, headline: { color: '#101828', fontSize: 19, lineHeight: 25, fontWeight: '800' }, platform: { color: '#667085', fontSize: 11, fontWeight: '800', letterSpacing: 1.1, marginTop: 3 }, copy: { color: '#344054', lineHeight: 21 }, sourceHeading: { color: '#344054', fontSize: 13, fontWeight: '800', marginTop: 5 }, source: { borderTopWidth: 1, borderColor: '#E6E8EF', paddingTop: 9, gap: 3 }, sourceTitle: { color: '#174EA6', fontSize: 14, fontWeight: '700' }, sourceUrl: { color: '#667085', fontSize: 12 }, sourceMeta: { color: '#98A2B3', fontSize: 11 },
});
