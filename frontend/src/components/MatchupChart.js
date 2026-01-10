/*
import React, { useEffect, useState } from 'react';
import { io } from 'socket.io-client';
import {
  ResponsiveContainer,
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Tooltip,
  LabelList,
} from 'recharts';
import { Card } from '@/components/ui/card';

// Initialize Socket.IO connection (point to your backend)
const defaultSocketURL =
  typeof window !== 'undefined'
    ? `${window.location.protocol}//${window.location.hostname}:4000`
    : 'http://localhost:4000';

const socket = io(process.env.NEXT_PUBLIC_WS_URL || defaultSocketURL);

/**
 * MatchupChart displays a real-time, interactive bar chart for a Matchup.
 * Props:
 *  - matchupId: unique identifier for the matchup in your backend
 */

/*
export default function MatchupChart({ matchupId }) {
  const [data, setData] = useState([]);

  useEffect(() => {
    if (!matchupId) return;
    // Join the socket room for this matchup
    socket.emit('joinMatchup', matchupId);

    // Listen for updates
    socket.on('matchupUpdate', (matchupData) => {
      // matchupData: [{ option: 'yes', votes: 4 }, ...]
      // Sort ascending by votes (so the largest bar is at the bottom)
      const sorted = [...matchupData].sort((a, b) => a.votes - b.votes);
      setData(sorted.map((d) => ({ name: d.option, value: d.votes })));
    });

    return () => {
      // Clean up listeners and leave the room
      socket.emit('leaveMatchup', matchupId);
      socket.off('matchupUpdate');
    };
  }, [matchupId]);

  return (
    <Card className="w-full h-full p-4">
      <ResponsiveContainer width="100%" height={300}>
        <BarChart
          data={data}
          layout="vertical"
          margin={{ top: 20, right: 40, bottom: 20, left: 20 }}
        >
          <XAxis type="number" hide />
          <YAxis type="category" dataKey="name" axisLine={false} tickLine={false} width={100} />
          <Tooltip formatter={(value) => `${value} votes`} />

          <Bar dataKey="value" barSize={20} radius={[10, 10, 10, 10]} animationDuration={500}>
            <LabelList dataKey="value" position="right" offset={8} />
          </Bar>
        </BarChart>
      </ResponsiveContainer>
    </Card>
  );
}
  */
