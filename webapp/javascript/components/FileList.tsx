import React, { useMemo } from 'react';

import { Maybe } from '@webapp/util/fp';
import { AllProfiles } from '@webapp/models/adhoc';
import TableUI, { useTableSort, BodyRow } from '@webapp/ui/Table';
import CheckIcon from './CheckIcon';
import styles from './FileList.module.scss';

const dateModifiedColName = 'updatedAt';
const fileNameColName = 'name';
const headRow = [
  { name: fileNameColName, label: 'Filename', sortable: 1 },
  {
    name: dateModifiedColName,
    label: 'Date Modified',
    sortable: 1,
    default: true,
  },
];

const getBodyRows = (
  sortedProfilesIds: AllProfiles[0][],
  onProfileSelected: (id: string) => void,
  selectedProfileId: Maybe<string>
): BodyRow[] => {
  return sortedProfilesIds.reduce((acc, profile) => {
    const isRowSelected = selectedProfileId.mapOr(
      false,
      (profId) => profId === profile.id
    );

    acc.push({
      cells: [
        {
          value: (
            <>
              {profile.name}
              {isRowSelected && <CheckIcon className={styles.checkIcon} />}
            </>
          ),
        },
        { value: profile.updatedAt },
      ],
      onClick: () => {
        // Optimize to not reload the same one
        if (
          selectedProfileId.isJust &&
          selectedProfileId.value === profile.id
        ) {
          return;
        }
        onProfileSelected(profile.id);
      },
      isRowSelected,
    });

    return acc;
  }, [] as BodyRow[]);
};

interface FileListProps {
  className?: string;
  profilesList: AllProfiles;
  onProfileSelected: (id: string) => void;
  selectedProfileId: Maybe<string>;
}

function FileList(props: FileListProps) {
  const {
    profilesList: profiles,
    onProfileSelected,
    className,
    selectedProfileId,
  } = props;

  const { sortBy, sortByDirection, ...rest } = useTableSort(headRow);
  const sortedProfilesIds = useMemo(() => {
    const m = sortByDirection === 'asc' ? 1 : -1;

    let sorted: AllProfiles[number][] = [];

    if (profiles) {
      const filesInfo = Object.values(profiles);

      switch (sortBy) {
        case fileNameColName:
          sorted = filesInfo.sort(
            (a, b) => m * a[sortBy].localeCompare(b[sortBy])
          );
          break;
        case dateModifiedColName:
          sorted = filesInfo.sort(
            (a, b) =>
              m *
              (new Date(a[sortBy]).getTime() - new Date(b[sortBy]).getTime())
          );
          break;
        default:
          sorted = filesInfo;
      }
    }

    return sorted;
  }, [profiles, sortBy, sortByDirection]);

  const tableBodyProps = profiles
    ? {
        type: 'filled' as const,
        bodyRows: getBodyRows(
          sortedProfilesIds,
          onProfileSelected,
          selectedProfileId
        ),
      }
    : { type: 'not-filled' as const, value: '' };

  return (
    <>
      <div className={`${styles.tableContainer} ${className}`}>
        <TableUI
          /* eslint-disable-next-line react/jsx-props-no-spreading */
          {...rest}
          sortBy={sortBy}
          sortByDirection={sortByDirection}
          table={{ headRow, ...tableBodyProps }}
        />
      </div>
    </>
  );
}

export default FileList;
