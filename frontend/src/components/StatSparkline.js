import React from 'react';
import { Area, AreaChart, ResponsiveContainer } from 'recharts';
import { motion } from 'framer-motion';

const StatSparkline = ({ data = [], color = '#f59e0b' }) => {
  if (!Array.isArray(data) || data.length === 0) {
    return null;
  }

  return (
    <motion.div
      className="home-stat-sparkline"
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      transition={{ duration: 0.25 }}
    >
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart data={data} margin={{ top: 6, right: 0, left: 0, bottom: 0 }}>
          <Area
            type="monotone"
            dataKey="value"
            stroke={color}
            strokeWidth={2}
            fill={color}
            fillOpacity={0.18}
            animationDuration={500}
          />
        </AreaChart>
      </ResponsiveContainer>
    </motion.div>
  );
};

export default StatSparkline;
