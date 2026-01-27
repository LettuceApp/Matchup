import React from 'react';
import { motion } from 'framer-motion';

const SkeletonCard = ({ lines = 2, className = '' }) => {
  return (
    <motion.div
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      transition={{ duration: 0.4 }}
      className={`rounded-2xl border border-slate-700/40 bg-slate-900/60 p-6 ${className}`}
    >
      <div className="h-4 w-28 rounded-full bg-slate-700/60 animate-pulse" />
      <div className="mt-4 flex flex-col gap-3">
        {Array.from({ length: lines }).map((_, index) => (
          <div
            key={index}
            className={`h-3 rounded-full bg-slate-700/40 animate-pulse ${index === 0 ? 'w-3/4' : 'w-full'}`}
          />
        ))}
      </div>
    </motion.div>
  );
};

export default SkeletonCard;
