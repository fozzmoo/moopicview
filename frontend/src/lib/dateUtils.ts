// Helper function to format date based on precision
export const formatDate = (dateStr: string, precision: string): string => {
  if (!dateStr) return 'Unknown date';

  // Parse the date string manually to avoid timezone issues
  const parts = dateStr.split('-');
  const year = parseInt(parts[0]);
  const month = parseInt(parts[1]) - 1; // JavaScript months are 0-indexed
  const day = parseInt(parts[2]);

  const monthNames = [
    'January', 'February', 'March', 'April', 'May', 'June',
    'July', 'August', 'September', 'October', 'November', 'December'
  ];

  switch (precision) {
    case 'year':
      return `${year}`;
    case 'month':
      return `${monthNames[month]} ${year}`;
    case 'exact':
    default:
      return `${monthNames[month]} ${day}, ${year}`;
  }
};