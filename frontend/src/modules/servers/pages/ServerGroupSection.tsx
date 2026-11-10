import clsx from 'clsx';
import { ChevronDown, ChevronRight, FolderTree } from 'lucide-react';
import type { ServerGroup } from './types';

interface GroupTreeProps {
  groups: ServerGroup[];
  level?: number;
  selectedGroupId: string | null;
  onSelectGroup: (id: string | null) => void;
}

export function GroupTree({ groups, level = 0, selectedGroupId, onSelectGroup }: GroupTreeProps) {
  return (
    <div className={level > 0 ? 'ml-4' : ''}>
      {groups.map((group) => (
        <div key={group.id}>
          <div
            className={clsx(
              'flex items-center gap-2 py-1.5 px-2 rounded cursor-pointer transition-colors text-sm',
              selectedGroupId === group.id
                ? 'bg-primary/10 text-primary'
                : 'hover:bg-background text-text-secondary',
            )}
            onClick={() => onSelectGroup(selectedGroupId === group.id ? null : group.id)}
          >
            {group.children && group.children.length > 0 ? (
              <ChevronDown className="w-3 h-3 flex-shrink-0" />
            ) : (
              <ChevronRight className="w-3 h-3 flex-shrink-0" />
            )}
            <FolderTree className="w-3.5 h-3.5 flex-shrink-0" />
            <span className="truncate">{group.name}</span>
            {group.server_count !== undefined && group.server_count > 0 && (
              <span className="ml-auto text-xs text-text-secondary">({group.server_count})</span>
            )}
          </div>
          {group.children && group.children.length > 0 && (
            <GroupTree
              groups={group.children}
              level={level + 1}
              selectedGroupId={selectedGroupId}
              onSelectGroup={onSelectGroup}
            />
          )}
        </div>
      ))}
    </div>
  );
}
