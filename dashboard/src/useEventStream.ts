import { useEffect, useReducer, useState } from 'react';
import { EVENT_TYPES, type CfaEvent } from './api/types';
import { initialState, reduce } from './state';

export type Connection = 'connecting' | 'live' | 'closed';

/**
 * Suit le flux SSE de l'API depuis le début (since=0). EventSource se reconnecte seul
 * avec Last-Event-ID ; reduce ignore les événements déjà appliqués. L'authentification
 * passe par le cookie de session posé par « cfa dashboard ».
 */
export function useEventStream() {
  const [state, dispatch] = useReducer(reduce, undefined, initialState);
  const [connection, setConnection] = useState<Connection>('connecting');

  useEffect(() => {
    const es = new EventSource('/events?since=0');
    const onEvent = (e: MessageEvent<string>) => dispatch(JSON.parse(e.data) as CfaEvent);
    for (const type of EVENT_TYPES) es.addEventListener(type, onEvent);
    es.onopen = () => setConnection('live');
    es.onerror = () => setConnection(es.readyState === EventSource.CLOSED ? 'closed' : 'connecting');
    return () => es.close();
  }, []);

  return { state, connection };
}
