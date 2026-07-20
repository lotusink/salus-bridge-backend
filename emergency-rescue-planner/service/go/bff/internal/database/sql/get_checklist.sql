-- ============================================================
-- Parameter
--   :disaster_type     TEXT   "fire" "earthquake" "flood"
--   :disability_type   TEXT   e.g. "Psychosocial Disability" "Multiple Sclerosis"
--                                  "Hearing Impairment" "Visual Impairment" "Cerebral Palsy" "Spinal Cord Injury" 
--                                  "Global Developmental Delay" "Down Syndrome" "Developmental Delay" "Autism" "Stroke"
--                                  "Other Sensory/Speech" "Other Neurological" "Intellectual Disability" "Other Physical"
-- ============================================================

SELECT
    item_name,
    reason
FROM pre_checklist
WHERE disaster_type = CAST(:disaster_type AS text)
  AND disability_type = CAST(:disability_type AS text)
ORDER BY
    (COALESCE(base_priority, 0) + COALESCE(disaster_weight, 0)) DESC,
    COALESCE(base_priority, 0) DESC,
    item_name ASC;