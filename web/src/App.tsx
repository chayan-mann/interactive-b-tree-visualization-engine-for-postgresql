import { useState } from 'react';
import { BPTreePanel } from './components/BPTreePanel';
import { PgLabPanel } from './components/PgLabPanel';

type Tab = 'tree' | 'pg';

export default function App() {
  const [tab, setTab] = useState<Tab>('tree');
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
