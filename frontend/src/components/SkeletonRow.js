import React from 'react';
import { motion } from 'framer-motion';

const SkeletonRow = () => (
  <motion.div
    initial={{ opacity: 0 }}
    animate={{ opacity: 1 }}
    transition={{ duration: 0.4 }}
    className="flex items-center justify-between gap-4 rounded-2xl border border-slate-700/40 bg-slate-900/60 p-4"
  >
    <div className="flex items-center gap-3">
      <div className="h-12 w-12 rounded-full bg-slate-700/50 animate-pulse" />
      <div className="flex flex-col gap-2">
        <div className="h-3 w-24 rounded-full bg-slate-700/50 animate-pulse" />
        <div className="h-3 w-40 rounded-full bg-slate-700/40 animate-pulse" />
      </div>
    </div>
    <div className="h-8 w-20 rounded-full bg-slate-700/40 animate-pulse" />
  </motion.div>
);

export default SkeletonRow;
