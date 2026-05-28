import { useEffect, useState } from 'react';
import { BPTreePanel } from './components/BPTreePanel';
import { PgLabPanel } from './components/PgLabPanel';

type Tab = 'tree' | 'pg';
const TAB_STORAGE_KEY = 'indexlab-active-tab';

function isTab(v: string | null): v is Tab {
  return v === 'tree' || v === 'pg';
}

function readInitialTab(): Tab {
  const params = new URLSearchParams(window.location.search);
  const queryTab = params.get('tab');
  if (isTab(queryTab)) {
    return queryTab;
  }
  const savedTab = window.localStorage.getItem(TAB_STORAGE_KEY);
  if (isTab(savedTab)) {
    return savedTab;
  }
  return 'tree';
}

export default function App() {
  const [tab, setTab] = useState<Tab>(readInitialTab);

  useEffect(() => {
    window.localStorage.setItem(TAB_STORAGE_KEY, tab);
    const url = new URL(window.location.href);
    url.searchParams.set('tab', tab);
    window.history.replaceState({}, '', url.toString());
  }, [tab]);

  return (
    <div style={{ maxWidth: 1280, margin: '0 auto', padding: '24px 28px' }}>
      <header
        style={{
          display: 'flex',
          alignItems: 'baseline',
          gap: 16,
          marginBottom: 18,
          flexWrap: 'wrap',
        }}
      >
        <h1 style={{ fontSize: 22 }}>IndexLab</h1>
        <span className="muted" style={{ fontSize: 13 }}>
          Visualize B+ trees and watch PostgreSQL pick a plan
        </span>
        <div style={{ marginLeft: 'auto', display: 'flex', gap: 6 }}>
          <button className={tab === 'tree' ? 'primary' : ''} onClick={() => setTab('tree')}>
            B+ tree
          </button>
          <button className={tab === 'pg' ? 'primary' : ''} onClick={() => setTab('pg')}>
            PostgreSQL lab
          </button>
        </div>
      </header>
      {tab === 'tree' ? <BPTreePanel /> : <PgLabPanel />}
    </div>
  );
}
