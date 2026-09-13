import { Stack } from 'expo-router';
export default function Layout() { return <Stack><Stack.Screen name="index" options={{ title: 'Newsroom' }} /><Stack.Screen name="agents/[id]" options={{ title: 'Conversation' }} /></Stack>; }
