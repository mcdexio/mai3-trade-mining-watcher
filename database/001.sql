-- DDL generated by Postico 1.5.19
-- Not all database features are supported. Do not use for backup.

-- Table Definition ----------------------------------------------

CREATE TABLE block (
                       id character varying(129) PRIMARY KEY,
                       number bigint NOT NULL,
                       timestamp bigint NOT NULL,
                       updated_at timestamp with time zone,
                       created_at timestamp with time zone
);

-- Indices -------------------------------------------------------

CREATE UNIQUE INDEX block_pkey ON block(id text_ops);
-- DDL generated by Postico 1.5.19
-- Not all database features are supported. Do not use for backup.

-- Table Definition ----------------------------------------------

CREATE TABLE progress (
                          table_name character varying(129) PRIMARY KEY,
                          epoch bigint NOT NULL,
                          "from" bigint NOT NULL,
                          "to" bigint NOT NULL,
                          checkpoint bigint,
                          updated_at timestamp with time zone,
                          created_at timestamp with time zone
);

-- Indices -------------------------------------------------------

CREATE UNIQUE INDEX progress_pkey ON progress(table_name text_ops);
-- DDL generated by Postico 1.5.19
-- Not all database features are supported. Do not use for backup.

-- Table Definition ----------------------------------------------

CREATE TABLE schedule (
                          epoch bigint NOT NULL,
                          start_time bigint NOT NULL,
                          end_time bigint NOT NULL,
                          weight_fee numeric(38,18) NOT NULL,
                          weight_oi numeric(38,18) NOT NULL,
                          weight_mcb numeric(38,18) NOT NULL
);

-- Indices -------------------------------------------------------

CREATE UNIQUE INDEX schedule_schedule_epoch_unique_idx ON schedule(epoch int8_ops);
-- DDL generated by Postico 1.5.19
-- Not all database features are supported. Do not use for backup.

-- Table Definition ----------------------------------------------

CREATE TABLE user_info (
                           trader character varying(128) NOT NULL,
                           epoch bigint NOT NULL,
                           init_fee numeric(38,18) NOT NULL,
                           acc_fee numeric(38,18) NOT NULL,
                           acc_pos_value numeric(38,18) NOT NULL,
                           cur_pos_value numeric(38,18) NOT NULL,
                           acc_stake_score numeric(38,18) NOT NULL,
                           cur_stake_score numeric(38,18) NOT NULL,
                           score numeric(38,18) NOT NULL,

                           timestamp bigint NOT NULL,
                           updated_at timestamp with time zone,
                           created_at timestamp with time zone,
                           PRIMARY KEY (trader, epoch)

);

CREATE TABLE snapshot (
                           trader character varying(128) NOT NULL,
                           epoch bigint NOT NULL,
                           timestamp bigint NOT NULL,

                           init_fee numeric(38,18) NOT NULL,
                           acc_fee numeric(38,18) NOT NULL,
                           acc_pos_value numeric(38,18) NOT NULL,
                           cur_pos_value numeric(38,18) NOT NULL,
                           acc_stake_score numeric(38,18) NOT NULL,
                           cur_stake_score numeric(38,18) NOT NULL,
                           score numeric(38,18) NOT NULL,

                           updated_at timestamp with time zone,
                           created_at timestamp with time zone,
                           PRIMARY KEY (trader, epoch, timestamp)

);

-- Indices -------------------------------------------------------

CREATE UNIQUE INDEX user_info_pkey ON user_info(trader, epoch);
