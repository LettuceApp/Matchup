import React, { useMemo } from 'react';
import {
  Bar,
  BarChart,
  Cell,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';
import { motion } from 'framer-motion';

const palette = ['#f59e0b', '#38bdf8', '#a78bfa', '#34d399'];

const MatchupChart = ({ matchup }) => {
  const data = useMemo(() => {
    const items = Array.isArray(matchup?.items) ? matchup.items : [];
    return items.map((item, index) => ({
      name: item.item || `Contender ${index + 1}`,
      votes: Number(item.votes ?? 0),
      fill: palette[index % palette.length],
    }));
  }, [matchup?.items]);

  if (!data.length) {
    return null;
  }

  const totalVotes = data.reduce((sum, entry) => sum + entry.votes, 0);

  return (
    <motion.div
      className="mt-4 rounded-2xl border border-slate-700/50 bg-slate-950/40 p-4"
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.3 }}
    >
      <div className="flex items-center justify-between text-xs font-semibold uppercase tracking-[0.2em] text-slate-400">
        <span>Votes snapshot</span>
        <span>{totalVotes} total</span>
      </div>
      <div className="mt-3 h-44 w-full">
        <ResponsiveContainer width="100%" height="100%">
          <BarChart data={data} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
            <XAxis
              dataKey="name"
              tickLine={false}
              axisLine={false}
              tick={{ fill: '#cbd5f5', fontSize: 11 }}
            />
            <YAxis
              tickLine={false}
              axisLine={false}
              tick={{ fill: '#94a3b8', fontSize: 10 }}
              allowDecimals={false}
            />
            <Tooltip
              cursor={{ fill: 'rgba(148,163,184,0.15)' }}
              contentStyle={{
                background: '#0f172a',
                border: '1px solid rgba(148,163,184,0.35)',
                borderRadius: 12,
                color: '#f8fafc',
                fontSize: 12,
              }}
              formatter={(value) => [`${value}`, 'Votes']}
            />
            <Bar dataKey="votes" radius={[10, 10, 10, 10]} animationDuration={600}>
              {data.map((entry) => (
                <Cell key={entry.name} fill={entry.fill} />
              ))}
            </Bar>
          </BarChart>
        </ResponsiveContainer>
      </div>
    </motion.div>
  );
};

export default MatchupChart;
